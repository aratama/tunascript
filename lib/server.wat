;; Server module functions implemented for GC backend.

(import "server" "sql_exec" (func $server._host_sql_exec (param i32 i32) (result externref)))
(import "server" "register_tables" (func $server._host_register_tables (param i32 i32)))
(import "server" "sql_query" (func $server._host_sql_query (param i32 i32 externref) (result externref)))
(import "server" "sql_fetch_one" (func $server._host_sql_fetch_one (param i32 i32 externref) (result externref)))
(import "server" "sql_fetch_optional" (func $server._host_sql_fetch_optional (param i32 i32 externref) (result externref)))
(import "server" "sql_execute" (func $server._host_sql_execute (param i32 i32 externref) (result externref)))
(import "server" "get_args" (func $server._host_get_args (result externref)))
(import "server" "get_env" (func $server._host_get_env (param externref) (result externref)))
(import "server" "gc" (func $server._host_gc))

(func $server.gc
  (call $server._host_gc)
)

(func $server.get_args (result anyref)
  (call $host.to_gc (call $server._host_get_args))
)

(func $server.get_env (param $name anyref) (result anyref)
  (call $host.to_gc
    (call $server._host_get_env (call $host.to_host (local.get $name)))
  )
)

(func $server.sql_exec (param $ptr i32) (param $len i32) (result anyref)
  (call $host.to_gc
    (call $server._host_sql_exec (local.get $ptr) (local.get $len))
  )
)

(func $server.register_tables (param $ptr i32) (param $len i32)
  (call $server._host_register_tables (local.get $ptr) (local.get $len))
)

(func $server.sql_query (param $ptr i32) (param $len i32) (param $params anyref) (result anyref)
  (call $host.to_gc
    (call $server._host_sql_query
      (local.get $ptr)
      (local.get $len)
      (call $host.to_host (local.get $params))
    )
  )
)

(func $server.sql_fetch_one (param $ptr i32) (param $len i32) (param $params anyref) (result anyref)
  (call $host.to_gc
    (call $server._host_sql_fetch_one
      (local.get $ptr)
      (local.get $len)
      (call $host.to_host (local.get $params))
    )
  )
)

(func $server.sql_fetch_optional (param $ptr i32) (param $len i32) (param $params anyref) (result anyref)
  (call $host.to_gc
    (call $server._host_sql_fetch_optional
      (local.get $ptr)
      (local.get $len)
      (call $host.to_host (local.get $params))
    )
  )
)

(func $server.sql_execute (param $ptr i32) (param $len i32) (param $params anyref) (result anyref)
  (call $host.to_gc
    (call $server._host_sql_execute
      (local.get $ptr)
      (local.get $len)
      (call $host.to_host (local.get $params))
    )
  )
)

;; sqlQuery wrapper (intrinsic fallback)
(func $server.sqlQuery (param $query anyref) (param $params anyref) (result anyref)
  (local $ptr i32)
  (local $len i32)
  (local.set $ptr (call $prelude._string_ptr (local.get $query)))
  (local.set $len (call $prelude._string_bytelen (local.get $query)))
  (call $server.sql_query (local.get $ptr) (local.get $len) (local.get $params))
)
