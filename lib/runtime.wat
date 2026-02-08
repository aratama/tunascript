;; Runtime module bridge for GC backend.

(import "runtime" "run_formatter" (func $runtime._host_run_formatter (param externref) (result externref)))
(import "runtime" "run_sandbox" (func $runtime._host_run_sandbox (param externref) (result externref)))

(func $runtime.run_formatter (param $source anyref) (result anyref)
  (call $host.to_gc
    (call $runtime._host_run_formatter
      (call $host.to_host (local.get $source))))
)

(func $runtime.run_sandbox (param $source anyref) (result anyref)
  (call $host.to_gc
    (call $runtime._host_run_sandbox
      (call $host.to_host (local.get $source))))
)
