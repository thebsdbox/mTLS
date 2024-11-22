# WIP

## How to Run

First build and run the eBPF program:
```
go generate
go build
sudo ./proxy
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

## Modified Architecture

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

The steps:

- Application on pod-01 does `connect()` to pod-02 (port80) `0.0.0.0:30000 -> 10.0.2.2:80`
- 🐝 modifies the socket`client 0.0.0.0:30000 -> 127.0.0.1:18000`
- Connection arrives `accept()` from `0.0.0.0:30000`, we get original destination address/port from socket
- We do a `connect()` to destination:18001 so (`10.0.2.2:18001`)
- We send the original port (80) as the first bit of data from pod-01 to pod-02 on port 18001
- Pod-02 creates an internal connection to `10.0.2.2:80`
- TLS the data between and YoLo

You can then inspect eBPF logs using `sudo cat /sys/kernel/debug/tracing/trace_pipe` to verify transparent proxy indeed intercepts the network traffic.

## Quick reload
```
kubectl delete -f ./deployment.yaml ;\
docker build -t kube-gateway/kube-gateway:v1 . ;\
kind load docker-image  kube-gateway/kube-gateway:v1 ;\
kubectl apply -f ./deployment.yaml
```

### 