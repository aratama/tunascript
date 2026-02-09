;; SQLite module functions implemented in WAT for host backend.
;; Values are kept as anyref in Wasm GC and converted at host call boundaries.

(import "host" "sqlite_db_open" (func $host.sqlite_db_open (param externref) (result externref)))
(import "server" "sql_exec" (func $sqlite._host_sql_exec (param i32 i32) (result externref)))
(import "server" "register_tables" (func $sqlite._host_register_tables (param i32 i32)))
(import "server" "sql_query" (func $sqlite._host_sql_query (param i32 i32 externref) (result externref)))
(import "server" "sql_fetch_one" (func $sqlite._host_sql_fetch_one (param i32 i32 externref) (result externref)))
(import "server" "sql_fetch_optional" (func $sqlite._host_sql_fetch_optional (param i32 i32 externref) (result externref)))
(import "server" "sql_execute" (func $sqlite._host_sql_execute (param i32 i32 externref) (result externref)))

(func $sqlite.sql_exec (param $ptr i32) (param $len i32) (result anyref)
  (call $interop.to_gc
    (call $sqlite._host_sql_exec (local.get $ptr) (local.get $len))
  )
)

(func $sqlite.register_tables (param $ptr i32) (param $len i32)
  (call $sqlite._host_register_tables (local.get $ptr) (local.get $len))
)

(func $sqlite.sql_query (param $ptr i32) (param $len i32) (param $params anyref) (result anyref)
  (call $interop.to_gc
    (call $sqlite._host_sql_query
      (local.get $ptr)
      (local.get $len)
      (call $interop.to_host (local.get $params))
    )
  )
)

(func $sqlite.sql_fetch_one (param $ptr i32) (param $len i32) (param $params anyref) (result anyref)
  (call $interop.to_gc
    (call $sqlite._host_sql_fetch_one
      (local.get $ptr)
      (local.get $len)
      (call $interop.to_host (local.get $params))
    )
  )
)

(func $sqlite.sql_fetch_optional (param $ptr i32) (param $len i32) (param $params anyref) (result anyref)
  (call $interop.to_gc
    (call $sqlite._host_sql_fetch_optional
      (local.get $ptr)
      (local.get $len)
      (call $interop.to_host (local.get $params))
    )
  )
)

(func $sqlite.sql_execute (param $ptr i32) (param $len i32) (param $params anyref) (result anyref)
  (call $interop.to_gc
    (call $sqlite._host_sql_execute
      (local.get $ptr)
      (local.get $len)
      (call $interop.to_host (local.get $params))
    )
  )
)

;; sqlQuery wrapper (intrinsic fallback)
(func $sqlite.sqlQuery (param $query anyref) (param $params anyref) (result anyref)
  (local $ptr i32)
  (local $len i32)
  (local.set $ptr (call $prelude._string_ptr (local.get $query)))
  (local.set $len (call $prelude._string_bytelen (local.get $query)))
  (call $sqlite.sql_query (local.get $ptr) (local.get $len) (local.get $params))
)

(func $sqlite.db_open (param $filename anyref) (result anyref)
  (call $interop.to_gc
    (call $host.sqlite_db_open
      (call $interop.to_host (local.get $filename))))
)
