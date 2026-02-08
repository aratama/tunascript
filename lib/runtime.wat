;; Runtime module bridge for GC backend.

(import "host" "runtime_run_formatter" (func $host.runtime_run_formatter (param externref) (result externref)))

(func $runtime.run_formatter (param $source anyref) (result anyref)
  (call $host.to_gc
    (call $host.runtime_run_formatter
      (call $host.to_host (local.get $source))))
)
