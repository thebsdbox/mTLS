// go:build ignore
#include "vmlinux.h"

#include <bpf/bpf_core_read.h>
#include <bpf/bpf_endian.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_tracing.h>

#define MAX_CONNECTIONS 20000
#define AF_INET 2 /* IP protocol family.  */
#define BPF_SOCK_OPS_ACTIVE_ESTABLISHED_CB 4

struct Config {
  __u32 proxy_addr;
  __u16 proxy_port;
  __u64 proxy_pid;
  __u32 network;
  __u16 mask;
  __u8 debug;
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

// struct pid_namespace *get_task_pid_ns(const struct task_struct *task);
// struct pid *get_task_pid_ptr(const struct task_struct *task,
//                              enum pid_type type);
// pid_t get_task_ns_pid(const struct task_struct *task, enum pid_type type);

// pid_t get_pid_nr_ns(struct pid *pid, struct pid_namespace *ns);
// pid_t get_ns_pid(void);

typedef struct pid_key {
  __u32 pid; // pid as seen by the userspace (for example, inside its container)
  __u32 ns;  // pids namespace for the process
} __attribute__((packed)) pid_key_t;

typedef struct pid_info_t {
  __u32 host_pid; // pid as seen by the root cgroup (and by BPF)
  __u32 user_pid; // pid as seen by the userspace (for example, inside its
                  // container)
  __u32 ns;       // pids namespace for the process
} __attribute__((packed)) pid_info;

static __always_inline void ns_pid_ppid(struct task_struct *task, int *pid,
                                        int *ppid, __u32 *pid_ns_id) {
  struct upid upid;

  unsigned int level = BPF_CORE_READ(task, nsproxy, pid_ns_for_children, level);
  struct pid *ns_pid =
      (struct pid *)BPF_CORE_READ(task, group_leader, thread_pid);
  bpf_probe_read_kernel(&upid, sizeof(upid), &ns_pid->numbers[level]);

  *pid = upid.nr;
  unsigned int p_level =
      BPF_CORE_READ(task, real_parent, nsproxy, pid_ns_for_children, level);

  struct pid *ns_ppid =
      (struct pid *)BPF_CORE_READ(task, real_parent, group_leader, thread_pid);
  bpf_probe_read_kernel(&upid, sizeof(upid), &ns_ppid->numbers[p_level]);
  *ppid = upid.nr;

  struct ns_common ns = BPF_CORE_READ(task, nsproxy, pid_ns_for_children, ns);
  *pid_ns_id = ns.inum;
}

// __hidden struct pid *get_task_pid_ptr(const struct task_struct *task,
//                                       enum pid_type type) {
//   // Returns the pid pointer of the given task. See get_task_pid_ptr for
//   // the kernel implementation.
//   return (type == PIDTYPE_PID) ? BPF_CORE_READ(task, thread_pid)
//                                : BPF_CORE_READ(task, signal, pids[type]);
// }

// __hidden struct pid_namespace *get_task_pid_ns(const struct task_struct
// *task,
//                                                enum pid_type type) {
//   struct pid_namespace *ns;
//   struct pid *p;
//   int level;

//   // See kernel function task_active_pid_ns in pid.c which calls into
//   // ns_of_pid. Returns the pid namespace of the given task.
//   if (!task)
//     task = (struct task_struct *)bpf_get_current_task();

//   if (!task)
//     return NULL;

//   p = get_task_pid_ptr(task, type);
//   if (!p)
//     return NULL;

//   level = BPF_CORE_READ(p, level);
//   ns = BPF_CORE_READ(p, numbers[level].ns);
//   return ns;
// }

// __hidden pid_t get_pid_nr_ns(struct pid *pid, struct pid_namespace *ns) {
//   int level, ns_level;
//   pid_t nr = 0;

//   /* This function implements the kernel equivalent pid_nr_ns in linux/pid.h
//    */
//   if (!pid || !ns)
//     return nr;

//   level = BPF_CORE_READ(pid, level);
//   ns_level = BPF_CORE_READ(ns, level);
//   if (ns_level <= level) {
//     struct upid upid;

//     upid = BPF_CORE_READ(pid, numbers[ns_level]);
//     if (upid.ns == ns)
//       nr = upid.nr;
//   }
//   return nr;
// }

// __hidden pid_t get_task_ns_pid(const struct task_struct *task) {
//   struct pid_namespace *ns;
//   struct pid *p;

//   if (!task)
//     task = (struct task_struct *)bpf_get_current_task();

//   ns = get_task_pid_ns(task, PIDTYPE_TGID);
//   p = get_task_pid_ptr(task, PIDTYPE_PID);
//   return get_pid_nr_ns(p, ns);
// }

// __hidden pid_t get_ns_pid(void) { return get_task_ns_pid(NULL); }