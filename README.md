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



`client 0.0.0.0:30000 -> 151.101.129.140:80`
`client 0.0.0.0:30000 -> 127.0.0.1:18000`
`Get original destination address/port from socket`
`Change dest port to 18001`
`0.0.0.0:30001 -> 151.101.129.140:18001`
`send original port`
`151.101.129.140:30002 -> 151.101.129.140:80`
`TLS between 0.0.0.0:30001 -> 151.101.129.140:18001`

You can then inspect eBPF logs using `sudo cat /sys/kernel/debug/tracing/trace_pipe` to verify transparent proxy indeed intercepts the network traffic.
