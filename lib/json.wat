;; JSON module bridge for GC backend.
;; Converts GC anyref values to host externref and back.

(import "host" "json_stringify" (func $host.json_stringify (param externref) (result externref)))
(import "host" "json_parse" (func $host.json_parse (param externref) (result externref)))
(import "host" "json_decode" (func $host.json_decode (param externref externref) (result externref)))

(func $json.stringify (param $value anyref) (result anyref)
  (call $host.to_gc
    (call $host.json_stringify
      (call $host.to_host (local.get $value))))
)

(func $json.parse (param $text anyref) (result anyref)
  (call $host.to_gc
    (call $host.json_parse
      (call $host.to_host (local.get $text))))
)

(func $json.decode (param $json anyref) (param $schema anyref) (result anyref)
  (call $host.to_gc
    (call $host.json_decode
      (call $host.to_host (local.get $json))
      (call $host.to_host (local.get $schema))))
)
