;; GC-oriented prelude runtime implemented in WAT.
;; Values live inside Wasm GC references (`anyref`) and strings are backed by UTF-8
;; byte slices in linear memory referenced as (ptr, len) pairs via `$Str`.

(type $Str (struct (field i32) (field i32)))
(type $ValI64 (struct (field i64)))
(type $ValF64 (struct (field f64)))
(type $ValBool (struct (field i32)))
(type $Undefined (struct))

(type $ArrData (array (mut anyref)))
(type $Arr (struct (field (mut (ref null $ArrData))) (field (mut i32))))

(type $ObjEntry (struct (field (mut anyref)) (field (mut anyref))))
(type $ObjData (array (mut (ref null $ObjEntry))))
(type $Obj (struct (field (mut (ref null $ObjData))) (field (mut i32))))

(import "wasi_snapshot_preview1" "fd_write"
  (func $wasi.fd_write (param i32 i32 i32 i32) (result i32)))

(global $heap_ptr (mut i32) (i32.const 0))
(global $heap_limit (mut i32) (i32.const 0))
(global $io_buf (mut i32) (i32.const 0))

(global $undef (mut anyref) (ref.null any))

(global $const_empty (mut anyref) (ref.null any))
(global $const_true (mut anyref) (ref.null any))
(global $const_false (mut anyref) (ref.null any))
(global $const_null (mut anyref) (ref.null any))
(global $const_undefined (mut anyref) (ref.null any))
(global $const_number (mut anyref) (ref.null any))
(global $const_array (mut anyref) (ref.null any))
(global $const_object (mut anyref) (ref.null any))
(global $const_value (mut anyref) (ref.null any))
(global $const_nan (mut anyref) (ref.null any))
(global $const_inf (mut anyref) (ref.null any))
(global $const_ninf (mut anyref) (ref.null any))
(global $const_newline (mut anyref) (ref.null any))
(global $const_type_key (mut anyref) (ref.null any))
(global $const_error_value (mut anyref) (ref.null any))
(global $const_message_key (mut anyref) (ref.null any))
(global $const_index_out_of_range (mut anyref) (ref.null any))

;; Reusable scratch buffer used by conversions to reduce temporary allocations.
(global $scratch_ptr (mut i32) (i32.const 0))
(global $scratch_cap (mut i32) (i32.const 0))

(data $d_true "true")
(data $d_false "false")
(data $d_null "null")
(data $d_undefined "undefined")
(data $d_number "[number]")
(data $d_array "[array]")
(data $d_object "[object]")
(data $d_value "[value]")
(data $d_nan "NaN")
(data $d_inf "Infinity")
(data $d_ninf "-Infinity")
(data $d_newline "\0a")
(data $d_type_key "type")
(data $d_error_value "error")
(data $d_message_key "message")
(data $d_index_out_of_range "index out of range")
(data $d_html_amp "&amp;")
(data $d_html_lt "&lt;")
(data $d_html_gt "&gt;")
(data $d_html_quot "&quot;")
(data $d_html_apos "&#39;")

(func $prelude._alloc (param $size i32) (result i32)
  (local $aligned i32)
  (local $old i32)
  (local $new_end i32)
  (local $needed i32)
  (local $pages i32)
  (local $grow_res i32)

  (call $prelude._ensure_runtime)

  (local.set $aligned
    (i32.and
      (i32.add (local.get $size) (i32.const 7))
      (i32.const -8)))

  (local.set $old (global.get $heap_ptr))
  (local.set $new_end (i32.add (local.get $old) (local.get $aligned)))

  (if (i32.gt_u (local.get $new_end) (global.get $heap_limit))
    (then
      (local.set $needed (i32.sub (local.get $new_end) (global.get $heap_limit)))
      (local.set $pages
        (i32.shr_u
          (i32.add (local.get $needed) (i32.const 65535))
          (i32.const 16)))
      (local.set $grow_res (memory.grow (local.get $pages)))
      (if (i32.eq (local.get $grow_res) (i32.const -1))
        (then
          unreachable
        )
      )
      (global.set $heap_limit
        (i32.add
          (global.get $heap_limit)
          (i32.shl (local.get $pages) (i32.const 16))))
    )
  )

  (global.set $heap_ptr (local.get $new_end))
  (local.get $old)
)

(func $prelude._new_const_true (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 4)))
  (memory.init $d_true (local.get $ptr) (i32.const 0) (i32.const 4))
  (struct.new $Str (local.get $ptr) (i32.const 4))
)

(func $prelude._new_const_false (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 5)))
  (memory.init $d_false (local.get $ptr) (i32.const 0) (i32.const 5))
  (struct.new $Str (local.get $ptr) (i32.const 5))
)

(func $prelude._new_const_null (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 4)))
  (memory.init $d_null (local.get $ptr) (i32.const 0) (i32.const 4))
  (struct.new $Str (local.get $ptr) (i32.const 4))
)

(func $prelude._new_const_undefined (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 9)))
  (memory.init $d_undefined (local.get $ptr) (i32.const 0) (i32.const 9))
  (struct.new $Str (local.get $ptr) (i32.const 9))
)

(func $prelude._new_const_number (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 8)))
  (memory.init $d_number (local.get $ptr) (i32.const 0) (i32.const 8))
  (struct.new $Str (local.get $ptr) (i32.const 8))
)

(func $prelude._new_const_array (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 7)))
  (memory.init $d_array (local.get $ptr) (i32.const 0) (i32.const 7))
  (struct.new $Str (local.get $ptr) (i32.const 7))
)

(func $prelude._new_const_object (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 8)))
  (memory.init $d_object (local.get $ptr) (i32.const 0) (i32.const 8))
  (struct.new $Str (local.get $ptr) (i32.const 8))
)

(func $prelude._new_const_value (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 7)))
  (memory.init $d_value (local.get $ptr) (i32.const 0) (i32.const 7))
  (struct.new $Str (local.get $ptr) (i32.const 7))
)

(func $prelude._new_const_nan (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 3)))
  (memory.init $d_nan (local.get $ptr) (i32.const 0) (i32.const 3))
  (struct.new $Str (local.get $ptr) (i32.const 3))
)

(func $prelude._new_const_inf (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 8)))
  (memory.init $d_inf (local.get $ptr) (i32.const 0) (i32.const 8))
  (struct.new $Str (local.get $ptr) (i32.const 8))
)

(func $prelude._new_const_ninf (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 9)))
  (memory.init $d_ninf (local.get $ptr) (i32.const 0) (i32.const 9))
  (struct.new $Str (local.get $ptr) (i32.const 9))
)

(func $prelude._new_const_newline (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 1)))
  (memory.init $d_newline (local.get $ptr) (i32.const 0) (i32.const 1))
  (struct.new $Str (local.get $ptr) (i32.const 1))
)

(func $prelude._new_const_type_key (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 4)))
  (memory.init $d_type_key (local.get $ptr) (i32.const 0) (i32.const 4))
  (struct.new $Str (local.get $ptr) (i32.const 4))
)

(func $prelude._new_const_error_value (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 5)))
  (memory.init $d_error_value (local.get $ptr) (i32.const 0) (i32.const 5))
  (struct.new $Str (local.get $ptr) (i32.const 5))
)

(func $prelude._new_const_message_key (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 7)))
  (memory.init $d_message_key (local.get $ptr) (i32.const 0) (i32.const 7))
  (struct.new $Str (local.get $ptr) (i32.const 7))
)

(func $prelude._new_const_index_out_of_range (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 18)))
  (memory.init $d_index_out_of_range (local.get $ptr) (i32.const 0) (i32.const 18))
  (struct.new $Str (local.get $ptr) (i32.const 18))
)

(func $prelude._ensure_runtime
  (if (i32.eqz (global.get $heap_limit))
    (then
      (global.set $heap_limit (i32.shl (memory.size) (i32.const 16)))
      (global.set $heap_ptr (global.get $heap_limit))

      (global.set $undef (struct.new $Undefined))
      (global.set $const_empty
        (struct.new $Str (call $prelude._alloc (i32.const 0)) (i32.const 0)))
      (global.set $const_true (call $prelude._new_const_true))
      (global.set $const_false (call $prelude._new_const_false))
      (global.set $const_null (call $prelude._new_const_null))
      (global.set $const_undefined (call $prelude._new_const_undefined))
      (global.set $const_number (call $prelude._new_const_number))
      (global.set $const_array (call $prelude._new_const_array))
      (global.set $const_object (call $prelude._new_const_object))
      (global.set $const_value (call $prelude._new_const_value))
      (global.set $const_nan (call $prelude._new_const_nan))
      (global.set $const_inf (call $prelude._new_const_inf))
      (global.set $const_ninf (call $prelude._new_const_ninf))
      (global.set $const_newline (call $prelude._new_const_newline))
      (global.set $const_type_key (call $prelude._new_const_type_key))
      (global.set $const_error_value (call $prelude._new_const_error_value))
      (global.set $const_message_key (call $prelude._new_const_message_key))
      (global.set $const_index_out_of_range (call $prelude._new_const_index_out_of_range))

      (global.set $io_buf (call $prelude._alloc (i32.const 12)))
    )
  )
)

(func $prelude._scratch_reserve (param $need i32) (result i32)
  (local $cap i32)
  (call $prelude._ensure_runtime)
  (if (i32.eqz (global.get $scratch_ptr))
    (then
      (local.set $cap (i32.const 32))
      (if (i32.gt_u (local.get $need) (local.get $cap))
        (then
          (local.set $cap
            (i32.and
              (i32.add (local.get $need) (i32.const 7))
              (i32.const -8)))
        )
      )
      (global.set $scratch_ptr (call $prelude._alloc (local.get $cap)))
      (global.set $scratch_cap (local.get $cap))
    )
  )
  (if (i32.gt_u (local.get $need) (global.get $scratch_cap))
    (then
      (local.set $cap
        (i32.and
          (i32.add (local.get $need) (i32.const 7))
          (i32.const -8)))
      (global.set $scratch_ptr (call $prelude._alloc (local.get $cap)))
      (global.set $scratch_cap (local.get $cap))
    )
  )
  (global.get $scratch_ptr)
)

(func $prelude._new_string_copy (param $ptr i32) (param $len i32) (result anyref)
  (local $dst i32)
  (call $prelude._ensure_runtime)
  (local.set $dst (call $prelude._alloc (local.get $len)))
  (memory.copy (local.get $dst) (local.get $ptr) (local.get $len))
  (struct.new $Str (local.get $dst) (local.get $len))
)

(func $prelude._new_string_owned (param $ptr i32) (param $len i32) (result anyref)
  (call $prelude._ensure_runtime)
  (struct.new $Str (local.get $ptr) (local.get $len))
)

(func $prelude._string_ptr (param $s anyref) (result i32)
  (struct.get $Str 0 (ref.cast (ref $Str) (local.get $s)))
)

(func $prelude._string_bytelen (param $s anyref) (result i32)
  (struct.get $Str 1 (ref.cast (ref $Str) (local.get $s)))
)

(func $prelude._write_bytes (param $ptr i32) (param $len i32)
  (call $prelude._ensure_runtime)
  (i32.store (global.get $io_buf) (local.get $ptr))
  (i32.store
    (i32.add (global.get $io_buf) (i32.const 4))
    (local.get $len))
  (drop
    (call $wasi.fd_write
      (i32.const 1)
      (global.get $io_buf)
      (i32.const 1)
      (i32.add (global.get $io_buf) (i32.const 8))))
)

(func $prelude._i64_to_string (param $v i64) (result anyref)
  (local $buf i32)
  (local $len i32)
  (local $neg i32)
  (local $mag i64)
  (local $digit i64)
  (local $tmp i64)
  (local $left i32)
  (local $right i32)
  (local $a i32)
  (local $b i32)

  (local.set $buf (call $prelude._scratch_reserve (i32.const 32)))
  (local.set $len (i32.const 0))
  (local.set $neg (i32.const 0))

  (if (i64.lt_s (local.get $v) (i64.const 0))
    (then
      (local.set $neg (i32.const 1))
      (local.set $mag (i64.sub (i64.const 0) (local.get $v)))
    )
    (else
      (local.set $mag (local.get $v))
    )
  )

  (if (i64.eq (local.get $mag) (i64.const 0))
    (then
      (i32.store8 (local.get $buf) (i32.const 48))
      (local.set $len (i32.const 1))
    )
    (else
      (block $done
        (loop $digits
          (local.set $digit
            (i64.rem_u (local.get $mag) (i64.const 10)))
          (i32.store8
            (i32.add (local.get $buf) (local.get $len))
            (i32.wrap_i64 (i64.add (local.get $digit) (i64.const 48))))
          (local.set $len (i32.add (local.get $len) (i32.const 1)))
          (local.set $mag
            (i64.div_u (local.get $mag) (i64.const 10)))
          (br_if $digits
            (i64.ne (local.get $mag) (i64.const 0)))
        )
      )
    )
  )

  (if (local.get $neg)
    (then
      (i32.store8
        (i32.add (local.get $buf) (local.get $len))
        (i32.const 45))
      (local.set $len (i32.add (local.get $len) (i32.const 1)))
    )
  )

  (local.set $left (i32.const 0))
  (local.set $right (i32.sub (local.get $len) (i32.const 1)))

  (block $rev_done
    (loop $rev
      (br_if $rev_done
        (i32.ge_u (local.get $left) (local.get $right)))
      (local.set $a
        (i32.load8_u (i32.add (local.get $buf) (local.get $left))))
      (local.set $b
        (i32.load8_u (i32.add (local.get $buf) (local.get $right))))
      (i32.store8
        (i32.add (local.get $buf) (local.get $left))
        (local.get $b))
      (i32.store8
        (i32.add (local.get $buf) (local.get $right))
        (local.get $a))
      (local.set $left (i32.add (local.get $left) (i32.const 1)))
      (local.set $right (i32.sub (local.get $right) (i32.const 1)))
      (br $rev)
    )
  )

  (call $prelude._new_string_copy (local.get $buf) (local.get $len))
)

(func $prelude._f64_to_string (param $v f64) (result anyref)
  (local $i i64)
  (if (f64.ne (local.get $v) (local.get $v))
    (then
      (return (global.get $const_nan))
    )
  )
  (if (f64.eq (local.get $v) (f64.const inf))
    (then
      (return (global.get $const_inf))
    )
  )
  (if (f64.eq (local.get $v) (f64.const -inf))
    (then
      (return (global.get $const_ninf))
    )
  )
  (local.set $i (i64.trunc_sat_f64_s (local.get $v)))
  (if (f64.eq (f64.convert_i64_s (local.get $i)) (local.get $v))
    (then
      (return (call $prelude._i64_to_string (local.get $i)))
    )
  )
  (global.get $const_number)
)

(func $prelude.toString (param $value anyref) (result anyref)
  (call $prelude._ensure_runtime)

  (if (ref.is_null (local.get $value))
    (then
      (return (global.get $const_null))
    )
  )
  (if (ref.test (ref $Undefined) (local.get $value))
    (then
      (return (global.get $const_undefined))
    )
  )
  (if (ref.test (ref $Str) (local.get $value))
    (then
      (return (local.get $value))
    )
  )
  (if (ref.test (ref $ValI64) (local.get $value))
    (then
      (return
        (call $prelude._i64_to_string
          (struct.get $ValI64 0 (ref.cast (ref $ValI64) (local.get $value)))))
    )
  )
  (if (ref.test (ref $ValF64) (local.get $value))
    (then
      (return
        (call $prelude._f64_to_string
          (struct.get $ValF64 0 (ref.cast (ref $ValF64) (local.get $value)))))
    )
  )
  (if (ref.test (ref $ValBool) (local.get $value))
    (then
      (if (i32.eqz
            (struct.get $ValBool 0 (ref.cast (ref $ValBool) (local.get $value))))
        (then
          (return (global.get $const_false))
        )
        (else
          (return (global.get $const_true))
        )
      )
    )
  )
  (if (ref.test (ref $Arr) (local.get $value))
    (then
      (return (global.get $const_array))
    )
  )
  (if (ref.test (ref $Obj) (local.get $value))
    (then
      (return (global.get $const_object))
    )
  )
  (global.get $const_value)
)

(func $prelude.log (param $value anyref)
  (local $text anyref)
  (local.set $text (call $prelude.toString (local.get $value)))
  (call $prelude._write_bytes
    (call $prelude._string_ptr (local.get $text))
    (call $prelude._string_bytelen (local.get $text)))
  (call $prelude._write_bytes
    (call $prelude._string_ptr (global.get $const_newline))
    (call $prelude._string_bytelen (global.get $const_newline)))
)

(func $prelude.stringLength (param $str anyref) (result i64)
  (call $prelude.str_len (local.get $str))
)

(func $prelude.str_from_utf8 (param $ptr i32) (param $length i32) (result anyref)
  (call $prelude._new_string_copy (local.get $ptr) (local.get $length))
)

(func $prelude.intern_string (param $ptr i32) (param $length i32) (result anyref)
  (call $prelude._new_string_copy (local.get $ptr) (local.get $length))
)

(func $prelude.str_concat (param $a anyref) (param $b anyref) (result anyref)
  (local $alen i32)
  (local $blen i32)
  (local $out_len i32)
  (local $out_ptr i32)
  (local.set $alen (call $prelude._string_bytelen (local.get $a)))
  (local.set $blen (call $prelude._string_bytelen (local.get $b)))
  (local.set $out_len (i32.add (local.get $alen) (local.get $blen)))
  (local.set $out_ptr (call $prelude._alloc (local.get $out_len)))
  (memory.copy
    (local.get $out_ptr)
    (call $prelude._string_ptr (local.get $a))
    (local.get $alen))
  (memory.copy
    (i32.add (local.get $out_ptr) (local.get $alen))
    (call $prelude._string_ptr (local.get $b))
    (local.get $blen))
  (call $prelude._new_string_owned (local.get $out_ptr) (local.get $out_len))
)

(func $prelude.str_eq (param $a anyref) (param $b anyref) (result i32)
  (local $alen i32)
  (local $blen i32)
  (local $aptr i32)
  (local $bptr i32)
  (local $i i32)

  (local.set $alen (call $prelude._string_bytelen (local.get $a)))
  (local.set $blen (call $prelude._string_bytelen (local.get $b)))
  (if (i32.ne (local.get $alen) (local.get $blen))
    (then
      (return (i32.const 0))
    )
  )

  (local.set $aptr (call $prelude._string_ptr (local.get $a)))
  (local.set $bptr (call $prelude._string_ptr (local.get $b)))
  (local.set $i (i32.const 0))

  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $alen)))
      (if
        (i32.ne
          (i32.load8_u (i32.add (local.get $aptr) (local.get $i)))
          (i32.load8_u (i32.add (local.get $bptr) (local.get $i))))
        (then
          (return (i32.const 0))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )

  (i32.const 1)
)

(func $prelude.str_len (param $str anyref) (result i64)
  (local $ptr i32)
  (local $len i32)
  (local $i i32)
  (local $count i32)
  (local $b i32)

  (local.set $ptr (call $prelude._string_ptr (local.get $str)))
  (local.set $len (call $prelude._string_bytelen (local.get $str)))
  (local.set $i (i32.const 0))
  (local.set $count (i32.const 0))

  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $b (i32.load8_u (i32.add (local.get $ptr) (local.get $i))))
      (if
        (i32.ne
          (i32.and (local.get $b) (i32.const 192))
          (i32.const 128))
        (then
          (local.set $count (i32.add (local.get $count) (i32.const 1)))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )

  (i64.extend_i32_u (local.get $count))
)

(func $prelude.val_from_i64 (param $v i64) (result anyref)
  (struct.new $ValI64 (local.get $v))
)

(func $prelude.val_from_f64 (param $v f64) (result anyref)
  (struct.new $ValF64 (local.get $v))
)

(func $prelude.val_from_bool (param $v i32) (result anyref)
  (if (result anyref)
    (i32.eqz (local.get $v))
    (then
      (struct.new $ValBool (i32.const 0))
    )
    (else
      (struct.new $ValBool (i32.const 1))
    )
  )
)

(func $prelude.val_null (result anyref)
  (ref.null any)
)

(func $prelude.val_undefined (result anyref)
  (call $prelude._ensure_runtime)
  (global.get $undef)
)

(func $prelude.val_to_i64 (param $value anyref) (result i64)
  (struct.get $ValI64 0 (ref.cast (ref $ValI64) (local.get $value)))
)

(func $prelude.val_to_f64 (param $value anyref) (result f64)
  (struct.get $ValF64 0 (ref.cast (ref $ValF64) (local.get $value)))
)

(func $prelude.val_to_bool (param $value anyref) (result i32)
  (struct.get $ValBool 0 (ref.cast (ref $ValBool) (local.get $value)))
)

(func $prelude._obj_find_index (param $obj (ref $Obj)) (param $key anyref) (result i32)
  (local $entries (ref null $ObjData))
  (local $len i32)
  (local $i i32)
  (local $entry (ref null $ObjEntry))
  (local $entry_key anyref)

  (local.set $entries (struct.get $Obj 0 (local.get $obj)))
  (local.set $len (struct.get $Obj 1 (local.get $obj)))
  (local.set $i (i32.const 0))

  (block $not_found
    (loop $loop
      (br_if $not_found (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $entry
        (array.get $ObjData (local.get $entries) (local.get $i)))
      (if (ref.is_null (local.get $entry))
        (then
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (local.set $entry_key
        (struct.get $ObjEntry 0 (ref.cast (ref $ObjEntry) (local.get $entry))))
      (if
        (call $prelude.str_eq (local.get $entry_key) (local.get $key))
        (then
          (return (local.get $i))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )

  (i32.const -1)
)

(func $prelude._arr_eq (param $a anyref) (param $b anyref) (result i32)
  (local $arr_a (ref $Arr))
  (local $arr_b (ref $Arr))
  (local $len i32)
  (local $i i32)
  (local $data_a (ref null $ArrData))
  (local $data_b (ref null $ArrData))

  (local.set $arr_a (ref.cast (ref $Arr) (local.get $a)))
  (local.set $arr_b (ref.cast (ref $Arr) (local.get $b)))
  (if
    (i32.ne
      (struct.get $Arr 1 (local.get $arr_a))
      (struct.get $Arr 1 (local.get $arr_b)))
    (then
      (return (i32.const 0))
    )
  )

  (local.set $len (struct.get $Arr 1 (local.get $arr_a)))
  (local.set $data_a (struct.get $Arr 0 (local.get $arr_a)))
  (local.set $data_b (struct.get $Arr 0 (local.get $arr_b)))
  (local.set $i (i32.const 0))

  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
      (if
        (i32.eqz
          (call $prelude.val_eq
            (array.get $ArrData (local.get $data_a) (local.get $i))
            (array.get $ArrData (local.get $data_b) (local.get $i))))
        (then
          (return (i32.const 0))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )

  (i32.const 1)
)

(func $prelude._obj_eq (param $a anyref) (param $b anyref) (result i32)
  (local $obj_a (ref $Obj))
  (local $obj_b (ref $Obj))
  (local $len i32)
  (local $i i32)
  (local $entries_a (ref null $ObjData))
  (local $entries_b (ref null $ObjData))
  (local $entry_a (ref null $ObjEntry))
  (local $key anyref)
  (local $idx_b i32)
  (local $entry_b (ref null $ObjEntry))
  (local $val_a anyref)
  (local $val_b anyref)

  (local.set $obj_a (ref.cast (ref $Obj) (local.get $a)))
  (local.set $obj_b (ref.cast (ref $Obj) (local.get $b)))

  (if
    (i32.ne
      (struct.get $Obj 1 (local.get $obj_a))
      (struct.get $Obj 1 (local.get $obj_b)))
    (then
      (return (i32.const 0))
    )
  )

  (local.set $len (struct.get $Obj 1 (local.get $obj_a)))
  (local.set $entries_a (struct.get $Obj 0 (local.get $obj_a)))
  (local.set $entries_b (struct.get $Obj 0 (local.get $obj_b)))
  (local.set $i (i32.const 0))

  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $entry_a
        (array.get $ObjData (local.get $entries_a) (local.get $i)))
      (if (ref.is_null (local.get $entry_a))
        (then
          (return (i32.const 0))
        )
      )
      (local.set $key
        (struct.get $ObjEntry 0 (ref.cast (ref $ObjEntry) (local.get $entry_a))))
      (local.set $idx_b
        (call $prelude._obj_find_index (local.get $obj_b) (local.get $key)))
      (if (i32.lt_s (local.get $idx_b) (i32.const 0))
        (then
          (return (i32.const 0))
        )
      )
      (local.set $entry_b
        (array.get $ObjData (local.get $entries_b) (local.get $idx_b)))
      (if (ref.is_null (local.get $entry_b))
        (then
          (return (i32.const 0))
        )
      )
      (local.set $val_a
        (struct.get $ObjEntry 1 (ref.cast (ref $ObjEntry) (local.get $entry_a))))
      (local.set $val_b
        (struct.get $ObjEntry 1 (ref.cast (ref $ObjEntry) (local.get $entry_b))))
      (if (i32.eqz (call $prelude.val_eq (local.get $val_a) (local.get $val_b)))
        (then
          (return (i32.const 0))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )

  (i32.const 1)
)

(func $prelude.val_kind (param $value anyref) (result i32)
  (if (ref.is_null (local.get $value))
    (then
      (return (i32.const 6))
    )
  )
  (if (ref.test (ref $Undefined) (local.get $value))
    (then
      (return (i32.const 7))
    )
  )
  (if (ref.test (ref $ValI64) (local.get $value))
    (then
      (return (i32.const 0))
    )
  )
  (if (ref.test (ref $ValF64) (local.get $value))
    (then
      (return (i32.const 1))
    )
  )
  (if (ref.test (ref $ValBool) (local.get $value))
    (then
      (return (i32.const 2))
    )
  )
  (if (ref.test (ref $Str) (local.get $value))
    (then
      (return (i32.const 3))
    )
  )
  (if (ref.test (ref $Obj) (local.get $value))
    (then
      (return (i32.const 4))
    )
  )
  (if (ref.test (ref $Arr) (local.get $value))
    (then
      (return (i32.const 5))
    )
  )
  (i32.const 7)
)

(func $prelude.val_eq (param $a anyref) (param $b anyref) (result i32)
  (if (ref.is_null (local.get $a))
    (then
      (if (result i32)
        (ref.is_null (local.get $b))
        (then (i32.const 1))
        (else (i32.const 0))
      )
      return
    )
  )
  (if (ref.is_null (local.get $b))
    (then
      (return (i32.const 0))
    )
  )

  (if (ref.test (ref $Undefined) (local.get $a))
    (then
      (if (result i32)
        (ref.test (ref $Undefined) (local.get $b))
        (then (i32.const 1))
        (else (i32.const 0))
      )
      return
    )
  )

  (if (ref.test (ref $ValI64) (local.get $a))
    (then
      (if (i32.eqz (ref.test (ref $ValI64) (local.get $b)))
        (then (return (i32.const 0)))
      )
      (if (result i32)
        (i64.eq
          (struct.get $ValI64 0 (ref.cast (ref $ValI64) (local.get $a)))
          (struct.get $ValI64 0 (ref.cast (ref $ValI64) (local.get $b))))
        (then (i32.const 1))
        (else (i32.const 0))
      )
      return
    )
  )

  (if (ref.test (ref $ValF64) (local.get $a))
    (then
      (if (i32.eqz (ref.test (ref $ValF64) (local.get $b)))
        (then (return (i32.const 0)))
      )
      (if (result i32)
        (f64.eq
          (struct.get $ValF64 0 (ref.cast (ref $ValF64) (local.get $a)))
          (struct.get $ValF64 0 (ref.cast (ref $ValF64) (local.get $b))))
        (then (i32.const 1))
        (else (i32.const 0))
      )
      return
    )
  )

  (if (ref.test (ref $ValBool) (local.get $a))
    (then
      (if (i32.eqz (ref.test (ref $ValBool) (local.get $b)))
        (then (return (i32.const 0)))
      )
      (if (result i32)
        (i32.eq
          (struct.get $ValBool 0 (ref.cast (ref $ValBool) (local.get $a)))
          (struct.get $ValBool 0 (ref.cast (ref $ValBool) (local.get $b))))
        (then (i32.const 1))
        (else (i32.const 0))
      )
      return
    )
  )

  (if (ref.test (ref $Str) (local.get $a))
    (then
      (if (i32.eqz (ref.test (ref $Str) (local.get $b)))
        (then (return (i32.const 0)))
      )
      (return (call $prelude.str_eq (local.get $a) (local.get $b)))
    )
  )

  (if (ref.test (ref $Arr) (local.get $a))
    (then
      (if (i32.eqz (ref.test (ref $Arr) (local.get $b)))
        (then (return (i32.const 0)))
      )
      (return (call $prelude._arr_eq (local.get $a) (local.get $b)))
    )
  )

  (if (ref.test (ref $Obj) (local.get $a))
    (then
      (if (i32.eqz (ref.test (ref $Obj) (local.get $b)))
        (then (return (i32.const 0)))
      )
      (return (call $prelude._obj_eq (local.get $a) (local.get $b)))
    )
  )

  (i32.const 0)
)

(func $prelude.obj_new (param $count i32) (result anyref)
  (local $cap i32)
  (local.set $cap (local.get $count))
  (if (i32.lt_s (local.get $cap) (i32.const 0))
    (then
      (local.set $cap (i32.const 0))
    )
  )
  (struct.new $Obj
    (array.new_default $ObjData (local.get $cap))
    (i32.const 0))
)

(func $prelude.obj_set (param $obj anyref) (param $key anyref) (param $value anyref)
  (local $typed (ref $Obj))
  (local $entries (ref null $ObjData))
  (local $idx i32)
  (local $len i32)
  (local $cap i32)
  (local $new_cap i32)
  (local $new_entries (ref null $ObjData))
  (local $i i32)
  (local $entry (ref null $ObjEntry))

  (local.set $typed (ref.cast (ref $Obj) (local.get $obj)))
  (local.set $idx
    (call $prelude._obj_find_index (local.get $typed) (local.get $key)))

  (if (i32.ge_s (local.get $idx) (i32.const 0))
    (then
      (local.set $entries (struct.get $Obj 0 (local.get $typed)))
      (local.set $entry
        (array.get $ObjData (local.get $entries) (local.get $idx)))
      (struct.set $ObjEntry 1
        (ref.cast (ref $ObjEntry) (local.get $entry))
        (local.get $value))
      (return)
    )
  )

  (local.set $entries (struct.get $Obj 0 (local.get $typed)))
  (local.set $len (struct.get $Obj 1 (local.get $typed)))
  (local.set $cap (array.len (local.get $entries)))

  (if (i32.ge_u (local.get $len) (local.get $cap))
    (then
      (if (i32.eqz (local.get $cap))
        (then
          (local.set $new_cap (i32.const 1))
        )
        (else
          (local.set $new_cap (i32.mul (local.get $cap) (i32.const 2)))
        )
      )
      (local.set $new_entries (array.new_default $ObjData (local.get $new_cap)))
      (local.set $i (i32.const 0))
      (block $copy_done
        (loop $copy
          (br_if $copy_done (i32.ge_u (local.get $i) (local.get $len)))
          (array.set $ObjData
            (local.get $new_entries)
            (local.get $i)
            (array.get $ObjData (local.get $entries) (local.get $i)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $copy)
        )
      )
      (struct.set $Obj 0 (local.get $typed) (local.get $new_entries))
      (local.set $entries (local.get $new_entries))
    )
  )

  (array.set $ObjData
    (local.get $entries)
    (local.get $len)
    (struct.new $ObjEntry (local.get $key) (local.get $value)))
  (struct.set $Obj 1
    (local.get $typed)
    (i32.add (local.get $len) (i32.const 1)))
)

(func $prelude.obj_get (param $obj anyref) (param $key anyref) (result anyref)
  (local $typed (ref $Obj))
  (local $entries (ref null $ObjData))
  (local $idx i32)
  (local $entry (ref null $ObjEntry))

  (call $prelude._ensure_runtime)
  (local.set $typed (ref.cast (ref $Obj) (local.get $obj)))
  (local.set $idx
    (call $prelude._obj_find_index (local.get $typed) (local.get $key)))
  (if (i32.lt_s (local.get $idx) (i32.const 0))
    (then
      (return (global.get $const_empty))
    )
  )
  (local.set $entries (struct.get $Obj 0 (local.get $typed)))
  (local.set $entry
    (array.get $ObjData (local.get $entries) (local.get $idx)))
  (struct.get $ObjEntry 1 (ref.cast (ref $ObjEntry) (local.get $entry)))
)

(func $prelude.arr_new (param $count i32) (result anyref)
  (local $len i32)
  (local.set $len (local.get $count))
  (if (i32.lt_s (local.get $len) (i32.const 0))
    (then
      (local.set $len (i32.const 0))
    )
  )
  (struct.new $Arr
    (array.new_default $ArrData (local.get $len))
    (local.get $len))
)

(func $prelude.arr_set (param $arr anyref) (param $index i32) (param $value anyref)
  (local $typed (ref $Arr))
  (local $len i32)
  (local $data (ref null $ArrData))
  (local.set $typed (ref.cast (ref $Arr) (local.get $arr)))
  (local.set $len (struct.get $Arr 1 (local.get $typed)))
  (if
    (i32.or
      (i32.lt_s (local.get $index) (i32.const 0))
      (i32.ge_u (local.get $index) (local.get $len)))
    (then
      unreachable
    )
  )
  (local.set $data (struct.get $Arr 0 (local.get $typed)))
  (array.set $ArrData (local.get $data) (local.get $index) (local.get $value))
)

(func $prelude.arr_get (param $arr anyref) (param $index i32) (result anyref)
  (local $typed (ref $Arr))
  (local $len i32)
  (local $data (ref null $ArrData))
  (local.set $typed (ref.cast (ref $Arr) (local.get $arr)))
  (local.set $len (struct.get $Arr 1 (local.get $typed)))
  (if
    (i32.or
      (i32.lt_s (local.get $index) (i32.const 0))
      (i32.ge_u (local.get $index) (local.get $len)))
    (then
      unreachable
    )
  )
  (local.set $data (struct.get $Arr 0 (local.get $typed)))
  (array.get $ArrData (local.get $data) (local.get $index))
)

(func $prelude.arr_get_result (param $arr anyref) (param $index i32) (result anyref)
  (local $typed (ref $Arr))
  (local $len i32)
  (local $data (ref null $ArrData))
  (local $err anyref)
  (call $prelude._ensure_runtime)
  (local.set $typed (ref.cast (ref $Arr) (local.get $arr)))
  (local.set $len (struct.get $Arr 1 (local.get $typed)))
  (if
    (i32.or
      (i32.lt_s (local.get $index) (i32.const 0))
      (i32.ge_u (local.get $index) (local.get $len)))
    (then
      (local.set $err (call $prelude.obj_new (i32.const 2)))
      (call $prelude.obj_set
        (local.get $err)
        (global.get $const_type_key)
        (global.get $const_error_value))
      (call $prelude.obj_set
        (local.get $err)
        (global.get $const_message_key)
        (global.get $const_index_out_of_range))
      (return (local.get $err))
    )
  )
  (local.set $data (struct.get $Arr 0 (local.get $typed)))
  (array.get $ArrData (local.get $data) (local.get $index))
)

(func $prelude.arr_len (param $arr anyref) (result i32)
  (struct.get $Arr 1 (ref.cast (ref $Arr) (local.get $arr)))
)

(func $prelude.arr_join (param $arr anyref) (result anyref)
  (local $typed (ref $Arr))
  (local $data (ref null $ArrData))
  (local $len i32)
  (local $i i32)
  (local $total i32)
  (local $elem anyref)
  (local $ptr i32)
  (local $out_ptr i32)
  (local $out_i i32)
  (local $elem_len i32)

  (call $prelude._ensure_runtime)
  (local.set $typed (ref.cast (ref $Arr) (local.get $arr)))
  (local.set $data (struct.get $Arr 0 (local.get $typed)))
  (local.set $len (struct.get $Arr 1 (local.get $typed)))
  (local.set $i (i32.const 0))
  (local.set $total (i32.const 0))

  (block $sum_done
    (loop $sum
      (br_if $sum_done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $elem (array.get $ArrData (local.get $data) (local.get $i)))
      (if (ref.test (ref $Str) (local.get $elem))
        (then
          (local.set $total
            (i32.add
              (local.get $total)
              (call $prelude._string_bytelen (local.get $elem))))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $sum)
    )
  )

  (if (i32.eqz (local.get $total))
    (then
      (return (global.get $const_empty))
    )
  )

  (local.set $out_ptr (call $prelude._alloc (local.get $total)))
  (local.set $out_i (i32.const 0))
  (local.set $i (i32.const 0))

  (block $copy_done
    (loop $copy
      (br_if $copy_done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $elem (array.get $ArrData (local.get $data) (local.get $i)))
      (if (ref.test (ref $Str) (local.get $elem))
        (then
          (local.set $elem_len (call $prelude._string_bytelen (local.get $elem)))
          (memory.copy
            (i32.add (local.get $out_ptr) (local.get $out_i))
            (call $prelude._string_ptr (local.get $elem))
            (local.get $elem_len))
          (local.set $out_i (i32.add (local.get $out_i) (local.get $elem_len)))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $copy)
    )
  )

  (call $prelude._new_string_owned (local.get $out_ptr) (local.get $total))
)

(func $prelude.call_fn (param $fn anyref) (param $args anyref) (result anyref)
  (call $__call_fn_dispatch (local.get $fn) (local.get $args))
)

(func $prelude.escape_html_attr (param $value anyref) (result anyref)
  (local $ptr i32)
  (local $len i32)
  (local $i i32)
  (local $b i32)
  (local $out_len i32)
  (local $out_ptr i32)
  (local $out_i i32)

  (call $prelude._ensure_runtime)
  (local.set $ptr (call $prelude._string_ptr (local.get $value)))
  (local.set $len (call $prelude._string_bytelen (local.get $value)))
  (local.set $i (i32.const 0))
  (local.set $out_len (i32.const 0))

  (block $len_done
    (loop $len_loop
      (br_if $len_done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $b (i32.load8_u (i32.add (local.get $ptr) (local.get $i))))
      (if (i32.eq (local.get $b) (i32.const 38))
        (then
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 5)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $len_loop)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 60))
        (then
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 4)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $len_loop)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 62))
        (then
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 4)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $len_loop)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 34))
        (then
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 6)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $len_loop)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 39))
        (then
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 5)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $len_loop)
        )
      )
      (local.set $out_len (i32.add (local.get $out_len) (i32.const 1)))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $len_loop)
    )
  )

  (if (i32.eq (local.get $out_len) (local.get $len))
    (then
      (return (local.get $value))
    )
  )

  (local.set $out_ptr (call $prelude._alloc (local.get $out_len)))
  (local.set $i (i32.const 0))
  (local.set $out_i (i32.const 0))

  (block $copy_done
    (loop $copy
      (br_if $copy_done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $b (i32.load8_u (i32.add (local.get $ptr) (local.get $i))))
      (if (i32.eq (local.get $b) (i32.const 38))
        (then
          (memory.init $d_html_amp
            (i32.add (local.get $out_ptr) (local.get $out_i))
            (i32.const 0)
            (i32.const 5))
          (local.set $out_i (i32.add (local.get $out_i) (i32.const 5)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $copy)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 60))
        (then
          (memory.init $d_html_lt
            (i32.add (local.get $out_ptr) (local.get $out_i))
            (i32.const 0)
            (i32.const 4))
          (local.set $out_i (i32.add (local.get $out_i) (i32.const 4)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $copy)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 62))
        (then
          (memory.init $d_html_gt
            (i32.add (local.get $out_ptr) (local.get $out_i))
            (i32.const 0)
            (i32.const 4))
          (local.set $out_i (i32.add (local.get $out_i) (i32.const 4)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $copy)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 34))
        (then
          (memory.init $d_html_quot
            (i32.add (local.get $out_ptr) (local.get $out_i))
            (i32.const 0)
            (i32.const 6))
          (local.set $out_i (i32.add (local.get $out_i) (i32.const 6)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $copy)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 39))
        (then
          (memory.init $d_html_apos
            (i32.add (local.get $out_ptr) (local.get $out_i))
            (i32.const 0)
            (i32.const 5))
          (local.set $out_i (i32.add (local.get $out_i) (i32.const 5)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $copy)
        )
      )
      (i32.store8 (i32.add (local.get $out_ptr) (local.get $out_i)) (local.get $b))
      (local.set $out_i (i32.add (local.get $out_i) (i32.const 1)))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $copy)
    )
  )

  (call $prelude._new_string_owned (local.get $out_ptr) (local.get $out_len))
)
