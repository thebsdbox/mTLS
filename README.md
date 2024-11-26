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

### TLS in action

#### Without the sidecar

The original port of `9000` is still being send cleartext traffic.

```
    10.0.0.227.35928 > 10.0.1.54.9000: Flags [P.], cksum 0x1650 (incorrect -> 0xd116), seq 153:170, ack 1, win 507, options [nop,nop,TS val 1710156213 ecr 1761501942], length 17
	0x0000:  4500 0045 8b67 4000 4006 9933 0a00 00e3  E..E.g@.@..3....
	0x0010:  0a00 0136 8c58 2328 ed78 5fcb 31aa 5b9b  ...6.X#(.x_.1.[.
	0x0020:  8018 01fb 1650 0000 0101 080a 65ee e9b5  .....P......e...
	0x0030:  68fe 62f6 4865 6c6c 6f20 6672 6f6d 2070  h.b.Hello.from.p
	0x0040:  6f64 2d30 31                             od-01
```

#### With the sidecar

We can see that the destination port has been changed to the TLS port `18443`. 
```
    10.0.0.196.51740 > 10.0.1.132.18443: Flags [P.], cksum 0x1695 (incorrect -> 0xef2a), seq 1740:1779, ack 1827, win 502, options [nop,nop,TS val 3093655397 ecr 4140148653], length 39
	0x0000:  4500 005b 7b63 4000 4006 a8f2 0a00 00c4  E..[{c@.@.......
	0x0010:  0a00 0184 ca1c 480b 8a63 4d53 f134 4176  ......H..cMS.4Av
	0x0020:  8018 01f6 1695 0000 0101 080a b865 6f65  .............eoe
	0x0030:  f6c5 a7ad 1703 0300 2244 536d cf88 3385  ........"DSm..3.
	0x0040:  263d d632 3795 b6b7 76c4 177d efee 9331  &=.27...v..}...1
	0x0050:  2dcb 7c3e 5c16 7af6 9164 eb              -.|>\.z..d.
```

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
