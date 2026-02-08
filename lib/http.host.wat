;; HTTP module functions implemented in WAT for host backend.
;; Values are kept as anyref in Wasm GC and converted at host call boundaries.

(import "host" "http_create_server" (func $host.http_create_server (result externref)))
(import "host" "http_add_route" (func $host.http_add_route (param externref externref i32 i32 externref)))
(import "host" "http_listen" (func $host.http_listen (param externref externref)))
(import "host" "http_response_text" (func $host.http_response_text (param i32 i32) (result externref)))
(import "host" "http_response_text_str" (func $host.http_response_text_str (param externref) (result externref)))
(import "host" "http_response_html" (func $host.http_response_html (param i32 i32) (result externref)))
(import "host" "http_response_html_str" (func $host.http_response_html_str (param externref) (result externref)))
(import "host" "http_response_json" (func $host.http_response_json (param externref) (result externref)))
(import "host" "http_response_redirect" (func $host.http_response_redirect (param i32 i32) (result externref)))
(import "host" "http_response_redirect_str" (func $host.http_response_redirect_str (param externref) (result externref)))
(import "host" "http_get_path" (func $host.http_get_path (param externref) (result externref)))
(import "host" "http_get_method" (func $host.http_get_method (param externref) (result externref)))

(data $d_star "*")

(func $http._str_star (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 1)))
  (memory.init $d_star (local.get $ptr) (i32.const 0) (i32.const 1))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 1))
)

(func $http.http_create_server (result anyref)
  (call $host.to_gc
    (call $host.http_create_server))
)

(func $http.http_add_route (param $server anyref) (param $method anyref) (param $path_ptr i32) (param $path_len i32) (param $handler anyref)
  (call $host.http_add_route
    (call $host.to_host (local.get $server))
    (call $host.to_host (local.get $method))
    (local.get $path_ptr)
    (local.get $path_len)
    (call $host.to_host (local.get $handler)))
)

(func $http.http_listen (param $server anyref) (param $port anyref)
  (call $host.http_listen
    (call $host.to_host (local.get $server))
    (call $host.to_host (local.get $port)))
)

(func $http.http_response_text (param $text_ptr i32) (param $text_len i32) (result anyref)
  (call $host.to_gc
    (call $host.http_response_text (local.get $text_ptr) (local.get $text_len)))
)

(func $http.http_response_text_str (param $text anyref) (result anyref)
  (call $host.to_gc
    (call $host.http_response_text_str
      (call $host.to_host (local.get $text))))
)

(func $http.http_response_html (param $html_ptr i32) (param $html_len i32) (result anyref)
  (call $host.to_gc
    (call $host.http_response_html (local.get $html_ptr) (local.get $html_len)))
)

(func $http.http_response_html_str (param $html anyref) (result anyref)
  (call $host.to_gc
    (call $host.http_response_html_str
      (call $host.to_host (local.get $html))))
)

(func $http.http_response_json (param $data anyref) (result anyref)
  (call $host.to_gc
    (call $host.http_response_json
      (call $host.to_host (local.get $data))))
)

(func $http.http_response_redirect (param $url_ptr i32) (param $url_len i32) (result anyref)
  (call $host.to_gc
    (call $host.http_response_redirect (local.get $url_ptr) (local.get $url_len)))
)

(func $http.http_response_redirect_str (param $url anyref) (result anyref)
  (call $host.to_gc
    (call $host.http_response_redirect_str
      (call $host.to_host (local.get $url))))
)

(func $http.http_get_path (param $req anyref) (result anyref)
  (call $host.to_gc
    (call $host.http_get_path
      (call $host.to_host (local.get $req))))
)

(func $http.http_get_method (param $req anyref) (result anyref)
  (call $host.to_gc
    (call $host.http_get_method
      (call $host.to_host (local.get $req))))
)

;; Public extern wrappers so declarations in lib/http.tuna remain available.

(func $http.create_server (result anyref)
  (call $http.http_create_server)
)

(func $http.add_route (param $server anyref) (param $path anyref) (param $handler anyref)
  (call $http.http_add_route
    (local.get $server)
    (call $http._str_star)
    (call $prelude._string_ptr (local.get $path))
    (call $prelude._string_bytelen (local.get $path))
    (local.get $handler))
)

(func $http.listen (param $server anyref) (param $port anyref)
  (call $http.http_listen (local.get $server) (local.get $port))
)

(func $http.responseText (param $text anyref) (result anyref)
  (call $http.http_response_text_str (local.get $text))
)

(func $http.response_html (param $html anyref) (result anyref)
  (call $http.http_response_html_str (local.get $html))
)

(func $http.responseJson (param $data anyref) (result anyref)
  (call $http.http_response_json (local.get $data))
)

(func $http.response_redirect (param $url anyref) (result anyref)
  (call $http.http_response_redirect_str (local.get $url))
)

(func $http.getPath (param $req anyref) (result anyref)
  (call $http.http_get_path (local.get $req))
)

(func $http.getMethod (param $req anyref) (result anyref)
  (call $http.http_get_method (local.get $req))
)
