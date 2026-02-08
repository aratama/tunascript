;; HTTP module functions implemented in WAT for GC backend.
;; No real socket server is started. `listen` executes GET / once and writes HTML to fd=3.

(import "wasi_snapshot_preview1" "fd_write"
  (func $http.wasi_fd_write (param i32 i32 i32 i32) (result i32)))

(global $http_inited (mut i32) (i32.const 0))
(global $http_iov_ptr (mut i32) (i32.const 0))
(global $http_root_any_handler (mut anyref) (ref.null any))
(global $http_root_get_handler (mut anyref) (ref.null any))

(data $d_empty "")
(data $d_root "/")
(data $d_star "*")
(data $d_get_lower "get")
(data $d_get_upper "GET")
(data $d_key_path "path")
(data $d_key_method "method")
(data $d_key_query "query")
(data $d_key_form "form")
(data $d_key_body "body")
(data $d_key_content_type "contentType")
(data $d_key_redirect_url "redirectUrl")
(data $d_ct_text "text/plain; charset=utf-8")
(data $d_ct_html "text/html; charset=utf-8")
(data $d_ct_json "application/json")
(data $d_ct_redirect "redirect")

(func $http._init
  (if (i32.eqz (global.get $http_inited))
    (then
      (global.set $http_iov_ptr (call $prelude._alloc (i32.const 12)))
      (global.set $http_inited (i32.const 1))
    )
  )
)

(func $http._str_empty (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 0)))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 0))
)

(func $http._str_root (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 1)))
  (memory.init $d_root (local.get $ptr) (i32.const 0) (i32.const 1))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 1))
)

(func $http._str_star (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 1)))
  (memory.init $d_star (local.get $ptr) (i32.const 0) (i32.const 1))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 1))
)

(func $http._str_get_lower (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 3)))
  (memory.init $d_get_lower (local.get $ptr) (i32.const 0) (i32.const 3))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 3))
)

(func $http._str_get_upper (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 3)))
  (memory.init $d_get_upper (local.get $ptr) (i32.const 0) (i32.const 3))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 3))
)

(func $http._str_key_path (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 4)))
  (memory.init $d_key_path (local.get $ptr) (i32.const 0) (i32.const 4))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 4))
)

(func $http._str_key_method (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 6)))
  (memory.init $d_key_method (local.get $ptr) (i32.const 0) (i32.const 6))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 6))
)

(func $http._str_key_query (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 5)))
  (memory.init $d_key_query (local.get $ptr) (i32.const 0) (i32.const 5))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 5))
)

(func $http._str_key_form (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 4)))
  (memory.init $d_key_form (local.get $ptr) (i32.const 0) (i32.const 4))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 4))
)

(func $http._str_key_body (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 4)))
  (memory.init $d_key_body (local.get $ptr) (i32.const 0) (i32.const 4))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 4))
)

(func $http._str_key_content_type (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 11)))
  (memory.init $d_key_content_type (local.get $ptr) (i32.const 0) (i32.const 11))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 11))
)

(func $http._str_key_redirect_url (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 11)))
  (memory.init $d_key_redirect_url (local.get $ptr) (i32.const 0) (i32.const 11))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 11))
)

(func $http._str_ct_text (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 25)))
  (memory.init $d_ct_text (local.get $ptr) (i32.const 0) (i32.const 25))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 25))
)

(func $http._str_ct_html (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 24)))
  (memory.init $d_ct_html (local.get $ptr) (i32.const 0) (i32.const 24))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 24))
)

(func $http._str_ct_json (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 16)))
  (memory.init $d_ct_json (local.get $ptr) (i32.const 0) (i32.const 16))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 16))
)

(func $http._str_ct_redirect (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 8)))
  (memory.init $d_ct_redirect (local.get $ptr) (i32.const 0) (i32.const 8))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 8))
)

(func $http._new_response (param $body anyref) (param $content_type anyref) (result anyref)
  (local $res anyref)
  (local.set $res (call $prelude.obj_new (i32.const 2)))
  (call $prelude.obj_set
    (local.get $res)
    (call $http._str_key_body)
    (local.get $body))
  (call $prelude.obj_set
    (local.get $res)
    (call $http._str_key_content_type)
    (local.get $content_type))
  (local.get $res)
)

(func $http._new_redirect_response (param $url anyref) (result anyref)
  (local $res anyref)
  (local.set $res (call $prelude.obj_new (i32.const 3)))
  (call $prelude.obj_set
    (local.get $res)
    (call $http._str_key_body)
    (call $http._str_empty))
  (call $prelude.obj_set
    (local.get $res)
    (call $http._str_key_content_type)
    (call $http._str_ct_redirect))
  (call $prelude.obj_set
    (local.get $res)
    (call $http._str_key_redirect_url)
    (local.get $url))
  (local.get $res)
)

(func $http._write_fd3 (param $text anyref)
  (local $ptr i32)
  (local $len i32)

  (local.set $ptr (call $prelude._string_ptr (local.get $text)))
  (local.set $len (call $prelude._string_bytelen (local.get $text)))

  (i32.store (global.get $http_iov_ptr) (local.get $ptr))
  (i32.store
    (i32.add (global.get $http_iov_ptr) (i32.const 4))
    (local.get $len))
  (drop
    (call $http.wasi_fd_write
      (i32.const 3)
      (global.get $http_iov_ptr)
      (i32.const 1)
      (i32.add (global.get $http_iov_ptr) (i32.const 8))))
)

(func $http.http_create_server (result anyref)
  (call $http._init)
  (call $prelude.obj_new (i32.const 0))
)

(func $http.http_add_route (param $server anyref) (param $method anyref) (param $path_ptr i32) (param $path_len i32) (param $handler anyref)
  (local $path_is_root i32)
  (local $is_get i32)
  (local $is_any i32)

  (call $http._init)

  (local.set $path_is_root (i32.const 0))
  (if (i32.eq (local.get $path_len) (i32.const 1))
    (then
      (if (i32.eq (i32.load8_u (local.get $path_ptr)) (i32.const 47))
        (then
          (local.set $path_is_root (i32.const 1))
        )
      )
    )
  )

  (if (i32.eqz (local.get $path_is_root))
    (then
      return
    )
  )

  (local.set $is_get (i32.const 0))
  (if (call $prelude.str_eq (local.get $method) (call $http._str_get_lower))
    (then
      (local.set $is_get (i32.const 1))
    )
  )
  (if (call $prelude.str_eq (local.get $method) (call $http._str_get_upper))
    (then
      (local.set $is_get (i32.const 1))
    )
  )
  (local.set $is_any (call $prelude.str_eq (local.get $method) (call $http._str_star)))

  (if (local.get $is_get)
    (then
      (global.set $http_root_get_handler (local.get $handler))
      return
    )
  )

  (if (local.get $is_any)
    (then
      (global.set $http_root_any_handler (local.get $handler))
    )
  )
)

(func $http.http_listen (param $server anyref) (param $port anyref)
  (local $handler anyref)
  (local $req anyref)
  (local $query anyref)
  (local $form anyref)
  (local $args anyref)
  (local $res anyref)
  (local $body anyref)

  (call $http._init)

  (local.set $handler (global.get $http_root_get_handler))
  (if (ref.is_null (local.get $handler))
    (then
      (local.set $handler (global.get $http_root_any_handler))
    )
  )

  (if (ref.is_null (local.get $handler))
    (then
      return
    )
  )

  (local.set $query (call $prelude.obj_new (i32.const 0)))
  (local.set $form (call $prelude.obj_new (i32.const 0)))
  (local.set $req (call $prelude.obj_new (i32.const 4)))

  (call $prelude.obj_set
    (local.get $req)
    (call $http._str_key_path)
    (call $http._str_root))
  (call $prelude.obj_set
    (local.get $req)
    (call $http._str_key_method)
    (call $http._str_get_upper))
  (call $prelude.obj_set
    (local.get $req)
    (call $http._str_key_query)
    (local.get $query))
  (call $prelude.obj_set
    (local.get $req)
    (call $http._str_key_form)
    (local.get $form))

  (local.set $args (call $prelude.arr_new (i32.const 1)))
  (call $prelude.arr_set (local.get $args) (i32.const 0) (local.get $req))

  (local.set $res (call $prelude.call_fn (local.get $handler) (local.get $args)))
  (local.set $body (call $prelude.obj_get (local.get $res) (call $http._str_key_body)))
  (call $http._write_fd3 (local.get $body))
)

(func $http.http_response_text (param $text_ptr i32) (param $text_len i32) (result anyref)
  (call $http._init)
  (call $http._new_response
    (call $prelude.str_from_utf8 (local.get $text_ptr) (local.get $text_len))
    (call $http._str_ct_text))
)

(func $http.http_response_text_str (param $text anyref) (result anyref)
  (call $http._init)
  (call $http._new_response
    (local.get $text)
    (call $http._str_ct_text))
)

(func $http.http_response_html (param $html_ptr i32) (param $html_len i32) (result anyref)
  (call $http._init)
  (call $http._new_response
    (call $prelude.str_from_utf8 (local.get $html_ptr) (local.get $html_len))
    (call $http._str_ct_html))
)

(func $http.http_response_html_str (param $html anyref) (result anyref)
  (call $http._init)
  (call $http._new_response
    (local.get $html)
    (call $http._str_ct_html))
)

(func $http.http_response_json (param $data anyref) (result anyref)
  (call $http._init)
  (call $http._new_response
    (call $http._str_empty)
    (call $http._str_ct_json))
)

(func $http.http_response_redirect (param $url_ptr i32) (param $url_len i32) (result anyref)
  (call $http._init)
  (call $http._new_redirect_response
    (call $prelude.str_from_utf8 (local.get $url_ptr) (local.get $url_len)))
)

(func $http.http_response_redirect_str (param $url anyref) (result anyref)
  (call $http._init)
  (call $http._new_redirect_response (local.get $url))
)

(func $http.http_get_path (param $req anyref) (result anyref)
  (call $http._init)
  (call $prelude.obj_get (local.get $req) (call $http._str_key_path))
)

(func $http.http_get_method (param $req anyref) (result anyref)
  (call $http._init)
  (call $prelude.obj_get (local.get $req) (call $http._str_key_method))
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
