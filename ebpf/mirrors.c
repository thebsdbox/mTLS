
#include <bpf/bpf_endian.h>
#include <linux/bpf.h>
#include <linux/in.h>
#include <linux/netfilter_ipv4.h>
#include <stddef.h>
#include <sys/socket.h>

#include "mirrors.h"

// This hook is triggered when a process (inside the cgroup where this is
// attached) calls the connect() syscall It redirect the connection to the
// transparent proxy but stores the original destination address and port in a
// map_socks
SEC("cgroup/connect4")
int cg_connect4(struct bpf_sock_addr *ctx) {
  // Only forward IPv4 TCP connections
  if (ctx->user_family != AF_INET)
    return 1;
  if (ctx->protocol != IPPROTO_TCP)
    return 1;

  __u32 key = 0;
  struct Config *conf = bpf_map_lookup_elem(&map_config, &key);
  if (!conf)
    return 1;

  // This field contains the IPv4 address passed to the connect() syscall
  // a.k.a. connect to this socket destination address and port
  __u32 dst_addr = bpf_ntohl(ctx->user_ip4);
  __u32 destination = bpf_ntohl(dst_addr);
  __u32 start = bpf_ntohl(conf->start_addr);
  __u32 end = bpf_ntohl(conf->end_addr);

  int mask = (-1) << (32 - conf->mask);
  // bpf_printk("hex 0x%x 0x%x 0x%x", bpf_htonl(ctx->user_ip4), mask,
  //            conf->network);
  // bpf_printk("IP in bounds %pI4 %pI4 %pI4", &destination, &start, &end);

  // If this packet is not part of the podCIDR range then return
  if ((bpf_htonl(ctx->user_ip4) & mask) != conf->network) {
    return 1;
  }
  bpf_printk("IP in bounds %pI4 %pI4 %pI4", &destination, &start, &end);

  // bpf_printk("IP out of bounds %u %u %u", destination, start, end);
  // bpf_printk("IP out of bounds %pI4 %pI4 %pI4", &destination, &start, &end);
  // if (destination < conf->start_addr || destination > conf->end_addr) {

  //   // bpf_printk("IP out of bounds %pI4 %pI4 %pI4", &destination, &start,
  //   // &end);
  //   return 1;
  // }

  // This field contains the port number passed to the connect() syscall
  __u16 dst_port = bpf_ntohl(ctx->user_port) >> 16;
  __u64 pid = (bpf_get_current_pid_tgid() >> 32);

  // This prevents the proxy from proxying itself

  bpf_printk("[%d vs %d] incoming %pI4:%d", pid, conf->proxy_pid, &destination,
             dst_port);
  if (pid == conf->proxy_pid)
    return 1;

  if (dst_port == 18001) {
    bpf_printk("Ignoring cluster to cluster");
    return 1;
  }
  // Unique identifier for the destination socket
  __u64 cookie = bpf_get_socket_cookie(ctx);

  // Store destination socket under cookie key
  struct Socket sock;
  __builtin_memset(&sock, 0, sizeof(sock));
  sock.dst_addr = dst_addr;
  sock.dst_port = dst_port;
  bpf_map_update_elem(&map_socks, &cookie, &sock, 0);

  // Redirect the connection to the proxy
  ctx->user_ip4 = bpf_htonl(conf->proxy_addr);
  // ctx->user_ip4 = bpf_htonl(0x7f000001);              // 127.0.0.1 == proxy
  // IP
  ctx->user_port = bpf_htonl(conf->proxy_port << 16); // Proxy port

  __u32 source = ctx->user_ip4;

  bpf_printk("New Connect() [%d] %pI4:%d to %pI4:%d", cookie, &destination,
             dst_port, &source, bpf_ntohs(ctx->user_port));

  return 1;
}

// This program is called whenever there's a socket operation on a particular
// cgroup (retransmit timeout, connection establishment, etc.) This is just to
// record client source address and port after succesful connection
// establishment to the proxy
SEC("sockops")
int cg_sock_ops(struct bpf_sock_ops *ctx) {
  // Only forward on IPv4 connections
  if (ctx->family != AF_INET)
    return 0;

  // Active socket with an established connection
  if (ctx->op == BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB) {
    __u64 cookie = bpf_get_socket_cookie(ctx);

    // Lookup the socket in the map for the corresponding cookie
    // In case the socket is present, store the source port and socket mapping
    struct Socket *sock = bpf_map_lookup_elem(&map_socks, &cookie);
    if (sock) {
      __u16 src_port = ctx->local_port;
      bpf_map_update_elem(&map_ports, &src_port, &cookie, 0);
    }
    __u32 destination = ctx->local_ip4;

    // bpf_printk("sockops hook successful %pI4:%d", &destination,
    //           ctx->local_port);
  }

  return 0;
}

// This is triggered when the proxy queries the original destination
// information through getsockopt SO_ORIGINAL_DST. This program uses the
// source port of the client to retrieve the socket's cookie from map_ports,
// and then from map_socks to get the original destination information, then
// establishes a connection with the original target and forwards the client's
// request.
SEC("cgroup/getsockopt")
int cg_sock_opt(struct bpf_sockopt *ctx) {
  // The SO_ORIGINAL_DST socket option is a specialized option used primarily
  // in the context of network address translation (NAT) and transparent
  // proxying. In a typical NAT or transparent proxy setup, incoming packets
  // are redirected from their original destination to a proxy server. The
  // proxy server, upon receiving the packets, often needs to know the
  // original destination address in order to handle the traffic
  // appropriately. This is where SO_ORIGINAL_DST comes into play.
  if (ctx->optname != SO_ORIGINAL_DST)
    return 1;
  // Only forward IPv4 TCP connections
  if (ctx->sk->family != AF_INET)
    return 1;
  if (ctx->sk->protocol != IPPROTO_TCP)
    return 1;

  // Get the clients source port
  // It's actually sk->dst_port because getsockopt() syscall with
  // SO_ORIGINAL_DST socket option is retrieving the original dst port of the
  // client so it's "querying" the destination port of the client
  __u16 src_port = bpf_ntohs(ctx->sk->dst_port);

  // Retrieve the socket cookie using the clients' src_port
  __u64 *cookie = bpf_map_lookup_elem(&map_ports, &src_port);
  if (!cookie)
    return 1;

  // Using the cookie (socket identifier), retrieve the original socket
  // (client connect to destination) from map_socks
  struct Socket *sock = bpf_map_lookup_elem(&map_socks, cookie);
  if (!sock)
    return 1;

  struct sockaddr_in *sa = ctx->optval;
  if ((void *)(sa + 1) > ctx->optval_end)
    return 1;

  // Establish a connection with the original destination target
  ctx->optlen = sizeof(*sa);
  sa->sin_family = ctx->sk->family;                // Address Family
  sa->sin_addr.s_addr = bpf_htonl(sock->dst_addr); // Destination Address
  sa->sin_port = bpf_htons(sock->dst_port);        // Destination Port
  ctx->retval = 0;
  __u32 address = sa->sin_addr.s_addr;
  bpf_printk("Redirecting %pI4:%d %d", &address, bpf_ntohs(sa->sin_port),
             ctx->sk->src_port);
  return 1;
}

char __LICENSE[] SEC("license") = "GPL";