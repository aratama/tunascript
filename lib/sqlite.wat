;; SQLite module functions implemented in WAT for GC backend.
;; db_open is a no-op and returns undefined.

(func $sqlite.db_open (param $filename anyref) (result anyref)
  (drop (local.get $filename))
  (call $prelude.val_undefined)
)
