# SMESH

It's a service mesh (kind of).

## How to Build and Run


```
go generate
go build -o smesh
sudo ./smesh
```

## Original Architecture
```
┌───────────┐          ┌───────────┐
│ Pod01     │          │ Pod01     │
│ 10.0.0.1  ┼────┬─────► 10.0.0.2  │
└───────────┘    │     └───────────┘
                 │                  
                 │                  
                 │                  
            CNI Magic 🧙🏻‍♂️
```

## Version (zero dot zero dot) one (simple tunneling)

This is the barebones of what I needed to achieve in order to transparently move traffic from one application to another.

```
┌─────────────────────────────────┐                     ┌─────────────────────────────────┐
│Pod-01                           │                     │                           Pod-02│
│10.0.0.1 x─x─x─x─► 10.0.2.2:80   │                     │     ┌────────────────►  10.0.2.2│
│   │  eBPF captures the socket   │                     │     │   :80                     │
│   │  Finds original destination │                     │     │                           │
│   │  Changes destination to lo  │                     │     │                           │
│   │                             │                     │     │                           │
│   ▼  Our listener sends         │                     │     │                           │
│127.0.0.1:18000                  │                     │0.0.0.0:18001                    │
│         │                       │                     │     ▲                           │
└─────────┼───────────────────────┘                     └─────┼───────────────────────────┘
          │                                                   │                            
          └───────────────────────────────────────────────────┘                            
            Uses original destination with a modified port                                 
```

The steps:

- Application on pod-01 does `connect()` to pod-02 (port80) `0.0.0.0:30000 -> 10.0.2.2:80`
- 🐝 modifies the socket`client 0.0.0.0:30000 -> 127.0.0.1:18000`
- Connection arrives `accept()` from `0.0.0.0:30000`, we get original destination address/port from socket
- We do a `connect()` to destination:18001 so (`10.0.2.2:18001`)
- We send the original port (80) as the first bit of data from pod-01 to pod-02 on port 18001
- Pod-02 creates an internal connection to `10.0.2.2:80`
- Send the data over and **YOLO**

### Observations

- It was really easy to break traffic as inside the pod I was seeing the network traffic from the whole KIND instance, so without guarrails in place I was tunnelling kubelet and the api-server etc.. and that was a mess.
- To ensure a situation where I did't try and redirect the traffic that we actually wanted to leave teh pod (so after we've captured it) we need to make sure we ignore any traffic from our pid. Sadly `__u64 pid = (bpf_get_current_pid_tgid() >> 32)` doesn't give us the `pid` inside the pod, but the one in the global namespace. Additionally `btf_bpf_get_ns_current_pid_tgid()` also doesn't work in a `cgroup` eBPF program, but luckily I found another way from spelunking around GitHub.
- The current implementation is messy but it works. 

## Version (zero dot zero dot) two (TLS)

Now pods we care about will mount `ca.crt` and their own `.crt/.key` and use those when communicating.
Additionally we have a program that "watches" pods, specifically the `update()` and when a pod gets an IP, then it will create the certs/secret will the required detail.

```
┌─────────────────────────────────┐                     ┌─────────────────────────────────┐
│Pod-01                           │                     │                           Pod-02│
│10.0.0.1 x─x─x─x─► 10.0.2.2:80   │                     │     ┌────────────────►  10.0.2.2│
│   │  eBPF captures the socket   │                     │     │   :80                     │
│   │  Finds original destination │                     │     │                           │
│   │  Changes destination to lo  │                     │     │                           │
│   │                             │                     │     │                           │
│   ▼  Our TLS listener sends     │                     │     │                           │
│127.0.0.1:18000                  │                     │0.0.0.0:18001                    │
│         │                       │                     │     ▲                           │
└─────────┼───────────────────────┘                     └─────┼───────────────────────────┘
          │                                                   │                            
          └────────────────────────🔐─────────────────────────┘                            
            Uses original destination with a modified port                                 
```
### Observations

- There is a delay as the sidecar will error as the secret usually doesn't exist in time.
- The eBPF code is still highly buggy :D 

## Troubleshooting
You can then inspect eBPF logs using `sudo cat /sys/kernel/debug/tracing/trace_pipe` to verify transparent proxy indeed intercepts the network traffic.

## Quick reload
```
kubectl delete -f ./deployment.yaml ;\
docker build -t kube-gateway/kube-gateway:v1 . ;\
kind load docker-image  kube-gateway/kube-gateway:v1 ;\
kubectl apply -f ./deployment.yaml
```

### 
