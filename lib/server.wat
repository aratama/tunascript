;; Server module functions implemented for GC backend.

(import "server" "get_args" (func $server._host_get_args (result externref)))
(import "server" "get_env" (func $server._host_get_env (param externref) (result externref)))
(import "server" "gc" (func $server._host_gc))

(func $server.gc
  (call $server._host_gc)
)

(func $server.get_args (result anyref)
  (call $interop.to_gc (call $server._host_get_args))
)

(func $server.get_env (param $name anyref) (result anyref)
  (call $interop.to_gc
    (call $server._host_get_env (call $interop.to_host (local.get $name)))
  )
)
