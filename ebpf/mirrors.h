// go:build ignore

#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#define MAX_CONNECTIONS 20000

struct Config {
  __u32 proxy_addr;
  __u16 proxy_port;
  __u64 proxy_pid;
};

struct Socket {
  __u32 src_addr;
  __u16 src_port;
  __u32 dst_addr;
  __u16 dst_port;
};

struct {
  __uint(type, BPF_MAP_TYPE_ARRAY);
  __uint(max_entries, 1);
  __type(key, __u32);
  __type(value, struct Config);
} map_config SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, MAX_CONNECTIONS);
  __type(key, __u32);
  __type(value, struct Socket);
} map_socks SEC(".maps");

struct {
  __uint(type, BPF_MAP_TYPE_HASH);
  __uint(max_entries, MAX_CONNECTIONS);
  __type(key, __u16);
  __type(value, __u64);
} map_ports SEC(".maps");