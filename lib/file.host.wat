;; File module functions implemented in WAT for host backend.
;; Values are kept as anyref in Wasm GC and converted at host call boundaries.

(import "host" "file_read_text" (func $host.file_read_text (param externref) (result externref)))
(import "host" "file_write_text" (func $host.file_write_text (param externref externref) (result externref)))
(import "host" "file_append_text" (func $host.file_append_text (param externref externref) (result externref)))
(import "host" "file_read_dir" (func $host.file_read_dir (param externref) (result externref)))
(import "host" "file_exists" (func $host.file_exists (param externref) (result i32)))

(func $file.read_text (param $path anyref) (result anyref)
  (call $host.to_gc
    (call $host.file_read_text
      (call $host.to_host (local.get $path))))
)

(func $file.write_text (param $path anyref) (param $content anyref) (result anyref)
  (call $host.to_gc
    (call $host.file_write_text
      (call $host.to_host (local.get $path))
      (call $host.to_host (local.get $content))))
)

(func $file.append_text (param $path anyref) (param $content anyref) (result anyref)
  (call $host.to_gc
    (call $host.file_append_text
      (call $host.to_host (local.get $path))
      (call $host.to_host (local.get $content))))
)

(func $file.read_dir (param $path anyref) (result anyref)
  (call $host.to_gc
    (call $host.file_read_dir
      (call $host.to_host (local.get $path))))
)

(func $file.exists (param $path anyref) (result i32)
  (call $host.file_exists
    (call $host.to_host (local.get $path)))
)
