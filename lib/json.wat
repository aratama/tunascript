;; JSON module bridge for GC backend.
;; Converts GC anyref values to externref and back.

(import "json" "stringify" (func $json._host_stringify (param externref) (result externref)))
(import "json" "parse" (func $json._host_parse (param externref) (result externref)))
(import "json" "decode" (func $json._host_decode (param externref externref) (result externref)))

(func $json.stringify (param $value anyref) (result anyref)
  (call $host.to_gc
    (call $json._host_stringify
      (call $host.to_host (local.get $value))))
)

(func $json.parse (param $text anyref) (result anyref)
  (call $host.to_gc
    (call $json._host_parse
      (call $host.to_host (local.get $text))))
)

(func $json.decode (param $json anyref) (param $schema anyref) (result anyref)
  (call $host.to_gc
    (call $json._host_decode
      (call $host.to_host (local.get $json))
      (call $host.to_host (local.get $schema))))
)
