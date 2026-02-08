;; SQLite module functions implemented in WAT for host backend.
;; Values are kept as anyref in Wasm GC and converted at host call boundaries.

(import "host" "sqlite_db_open" (func $host.sqlite_db_open (param externref) (result externref)))

(func $sqlite.db_open (param $filename anyref) (result anyref)
  (call $host.to_gc
    (call $host.sqlite_db_open
      (call $host.to_host (local.get $filename))))
)
