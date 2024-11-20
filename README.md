# WIP

## How to Run

First build and run the eBPF program:
```
go generate
go build
sudo ./proxy
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
