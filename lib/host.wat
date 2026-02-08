;; Host bridge helpers used by server.wat (GC backend).
;; These functions convert values between GC anyref and host externref.

(import "host" "val_from_i64" (func $host.val_from_i64 (param i64) (result externref)))
(import "host" "val_from_f64" (func $host.val_from_f64 (param f64) (result externref)))
(import "host" "val_from_bool" (func $host.val_from_bool (param i32) (result externref)))
(import "host" "val_null" (func $host.val_null (result externref)))
(import "host" "val_undefined" (func $host.val_undefined (result externref)))
(import "host" "val_to_i64" (func $host.val_to_i64 (param externref) (result i64)))
(import "host" "val_to_f64" (func $host.val_to_f64 (param externref) (result f64)))
(import "host" "val_to_bool" (func $host.val_to_bool (param externref) (result i32)))
(import "host" "val_kind" (func $host.val_kind (param externref) (result i32)))

(import "host" "str_from_utf8" (func $host.str_from_utf8 (param i32 i32) (result externref)))
(import "host" "str_byte_len" (func $host.str_byte_len (param externref) (result i32)))
(import "host" "str_copy" (func $host.str_copy (param externref i32 i32)))

(import "host" "arr_new" (func $host.arr_new (param i32) (result externref)))
(import "host" "arr_len" (func $host.arr_len (param externref) (result i32)))
(import "host" "arr_get" (func $host.arr_get (param externref i32) (result externref)))
(import "host" "arr_set" (func $host.arr_set (param externref i32 externref)))

(import "host" "obj_new" (func $host.obj_new (param i32) (result externref)))
(import "host" "obj_get" (func $host.obj_get (param externref externref) (result externref)))
(import "host" "obj_set" (func $host.obj_set (param externref externref externref)))
(import "host" "obj_keys" (func $host.obj_keys (param externref) (result externref)))

(func $host.to_host (param $value anyref) (result externref)
  (local $kind i32)
  (local $len i32)
  (local $ptr i32)
  (local $i i32)
  (local $out externref)
  (local $keys anyref)
  (local $key anyref)
  (local $val anyref)

  (local.set $kind (call $prelude.val_kind (local.get $value)))

  (if (i32.eq (local.get $kind) (i32.const 0))
    (then
      (return (call $host.val_from_i64 (call $prelude.val_to_i64 (local.get $value))))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 1))
    (then
      (return (call $host.val_from_f64 (call $prelude.val_to_f64 (local.get $value))))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 2))
    (then
      (return (call $host.val_from_bool (call $prelude.val_to_bool (local.get $value))))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 3))
    (then
      (local.set $ptr (call $prelude._string_ptr (local.get $value)))
      (local.set $len (call $prelude._string_bytelen (local.get $value)))
      (return (call $host.str_from_utf8 (local.get $ptr) (local.get $len)))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 5))
    (then
      (local.set $len (call $prelude.arr_len (local.get $value)))
      (local.set $out (call $host.arr_new (local.get $len)))
      (local.set $i (i32.const 0))
      (block $done
        (loop $loop
          (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
          (local.set $val (call $prelude.arr_get (local.get $value) (local.get $i)))
          (call $host.arr_set
            (local.get $out)
            (local.get $i)
            (call $host.to_host (local.get $val)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (return (local.get $out))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 4))
    (then
      (local.set $keys (call $prelude.obj_keys (local.get $value)))
      (local.set $len (call $prelude.arr_len (local.get $keys)))
      (local.set $out (call $host.obj_new (local.get $len)))
      (local.set $i (i32.const 0))
      (block $done
        (loop $loop
          (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
          (local.set $key (call $prelude.arr_get (local.get $keys) (local.get $i)))
          (local.set $val (call $prelude.obj_get (local.get $value) (local.get $key)))
          (call $host.obj_set
            (local.get $out)
            (call $host.to_host (local.get $key))
            (call $host.to_host (local.get $val)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (return (local.get $out))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 6))
    (then
      (return (call $host.val_null))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 7))
    (then
      (return (call $host.val_undefined))
    )
  )

  (call $host.val_undefined)
)

(func $host.to_gc (param $value externref) (result anyref)
  (local $kind i32)
  (local $len i32)
  (local $ptr i32)
  (local $i i32)
  (local $out anyref)
  (local $keys externref)
  (local $key externref)
  (local $val externref)

  (local.set $kind (call $host.val_kind (local.get $value)))

  (if (i32.eq (local.get $kind) (i32.const 0))
    (then
      (return (call $prelude.val_from_i64 (call $host.val_to_i64 (local.get $value))))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 1))
    (then
      (return (call $prelude.val_from_f64 (call $host.val_to_f64 (local.get $value))))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 2))
    (then
      (return (call $prelude.val_from_bool (call $host.val_to_bool (local.get $value))))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 3))
    (then
      (local.set $len (call $host.str_byte_len (local.get $value)))
      (local.set $ptr (call $prelude._scratch_reserve (local.get $len)))
      (call $host.str_copy (local.get $value) (local.get $ptr) (local.get $len))
      (return (call $prelude.str_from_utf8 (local.get $ptr) (local.get $len)))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 5))
    (then
      (local.set $len (call $host.arr_len (local.get $value)))
      (local.set $out (call $prelude.arr_new (local.get $len)))
      (local.set $i (i32.const 0))
      (block $done
        (loop $loop
          (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
          (local.set $val (call $host.arr_get (local.get $value) (local.get $i)))
          (call $prelude.arr_set
            (local.get $out)
            (local.get $i)
            (call $host.to_gc (local.get $val)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (return (local.get $out))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 4))
    (then
      (local.set $keys (call $host.obj_keys (local.get $value)))
      (local.set $len (call $host.arr_len (local.get $keys)))
      (local.set $out (call $prelude.obj_new (local.get $len)))
      (local.set $i (i32.const 0))
      (block $done
        (loop $loop
          (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
          (local.set $key (call $host.arr_get (local.get $keys) (local.get $i)))
          (local.set $val (call $host.obj_get (local.get $value) (local.get $key)))
          (call $prelude.obj_set
            (local.get $out)
            (call $host.to_gc (local.get $key))
            (call $host.to_gc (local.get $val)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (return (local.get $out))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 6))
    (then
      (return (call $prelude.val_null))
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 7))
    (then
      (return (call $prelude.val_undefined))
    )
  )

  (call $prelude.val_undefined)
)
