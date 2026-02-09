;; JSON module for GC backend.
;; stringify/toJSON/parse/decode are implemented in WAT.

(global $json_out_ptr (mut i32) (i32.const 0))
(global $json_out_len (mut i32) (i32.const 0))
(global $json_out_cap (mut i32) (i32.const 0))

(global $json_parse_ptr (mut i32) (i32.const 0))
(global $json_parse_end (mut i32) (i32.const 0))
(global $json_parse_err (mut i32) (i32.const 0))
(global $json_parse_err_msg (mut anyref) (ref.null any))

(global $json_decode_err (mut i32) (i32.const 0))
(global $json_decode_err_msg (mut anyref) (ref.null any))

(data $json_d_type "type")
(data $json_d_error "error")
(data $json_d_message "message")
(data $json_d_toJSON_expects_string "toJSON expects string")
(data $json_d_invalid_json "invalid json")
(data $json_d_decode_expects_schema_string "decode expects schema string")
(data $json_d_invalid_schema "invalid schema")
(data $json_d_colon_space ": ")
(data $json_d_dot ".")
(data $json_d_lbr "[")
(data $json_d_rbr "]")
(data $json_d_dollar "$")

(data $json_d_kind "kind")
(data $json_d_literal "literal")
(data $json_d_value "value")
(data $json_d_elem "elem")
(data $json_d_tuple "tuple")
(data $json_d_props "props")
(data $json_d_name "name")
(data $json_d_index "index")
(data $json_d_union "union")

(data $json_d_undefined_expected "undefined expected")
(data $json_d_null_expected "null expected")
(data $json_d_string_expected "string expected")
(data $json_d_string_literal_mismatch "string literal mismatch")
(data $json_d_boolean_expected "boolean expected")
(data $json_d_boolean_literal_mismatch "boolean literal mismatch")
(data $json_d_invalid_number "invalid number")
(data $json_d_integer_expected "integer expected")
(data $json_d_integer_out_of_range "integer out of range")
(data $json_d_integer_literal_mismatch "integer literal mismatch")
(data $json_d_number_expected "number expected")
(data $json_d_number_literal_mismatch "number literal mismatch")
(data $json_d_array_expected "array expected")
(data $json_d_tuple_length_mismatch "tuple length mismatch")
(data $json_d_missing_field "missing field")
(data $json_d_union_expected "union expected")
(data $json_d_unsupported_schema_kind "unsupported schema kind")
(data $json_d_object_expected "object expected")

(func $json._str_type (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 4)))
  (memory.init $json_d_type (local.get $ptr) (i32.const 0) (i32.const 4))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 4))
)

(func $json._str_error (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 5)))
  (memory.init $json_d_error (local.get $ptr) (i32.const 0) (i32.const 5))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 5))
)

(func $json._str_message (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 7)))
  (memory.init $json_d_message (local.get $ptr) (i32.const 0) (i32.const 7))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 7))
)

(func $json._msg_toJSON_expects_string (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 21)))
  (memory.init $json_d_toJSON_expects_string (local.get $ptr) (i32.const 0) (i32.const 21))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 21))
)

(func $json._msg_invalid_json (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 12)))
  (memory.init $json_d_invalid_json (local.get $ptr) (i32.const 0) (i32.const 12))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 12))
)

(func $json._msg_decode_expects_schema_string (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 28)))
  (memory.init $json_d_decode_expects_schema_string (local.get $ptr) (i32.const 0) (i32.const 28))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 28))
)

(func $json._msg_invalid_schema (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 14)))
  (memory.init $json_d_invalid_schema (local.get $ptr) (i32.const 0) (i32.const 14))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 14))
)

(func $json._msg_undefined_expected (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 18)))
  (memory.init $json_d_undefined_expected (local.get $ptr) (i32.const 0) (i32.const 18))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 18))
)

(func $json._msg_null_expected (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 13)))
  (memory.init $json_d_null_expected (local.get $ptr) (i32.const 0) (i32.const 13))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 13))
)

(func $json._msg_string_expected (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 15)))
  (memory.init $json_d_string_expected (local.get $ptr) (i32.const 0) (i32.const 15))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 15))
)

(func $json._msg_string_literal_mismatch (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 23)))
  (memory.init $json_d_string_literal_mismatch (local.get $ptr) (i32.const 0) (i32.const 23))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 23))
)

(func $json._msg_boolean_expected (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 16)))
  (memory.init $json_d_boolean_expected (local.get $ptr) (i32.const 0) (i32.const 16))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 16))
)

(func $json._msg_boolean_literal_mismatch (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 24)))
  (memory.init $json_d_boolean_literal_mismatch (local.get $ptr) (i32.const 0) (i32.const 24))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 24))
)

(func $json._msg_invalid_number (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 14)))
  (memory.init $json_d_invalid_number (local.get $ptr) (i32.const 0) (i32.const 14))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 14))
)

(func $json._msg_integer_expected (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 16)))
  (memory.init $json_d_integer_expected (local.get $ptr) (i32.const 0) (i32.const 16))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 16))
)

(func $json._msg_integer_out_of_range (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 20)))
  (memory.init $json_d_integer_out_of_range (local.get $ptr) (i32.const 0) (i32.const 20))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 20))
)

(func $json._msg_integer_literal_mismatch (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 24)))
  (memory.init $json_d_integer_literal_mismatch (local.get $ptr) (i32.const 0) (i32.const 24))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 24))
)

(func $json._msg_number_expected (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 15)))
  (memory.init $json_d_number_expected (local.get $ptr) (i32.const 0) (i32.const 15))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 15))
)

(func $json._msg_number_literal_mismatch (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 23)))
  (memory.init $json_d_number_literal_mismatch (local.get $ptr) (i32.const 0) (i32.const 23))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 23))
)

(func $json._msg_array_expected (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 14)))
  (memory.init $json_d_array_expected (local.get $ptr) (i32.const 0) (i32.const 14))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 14))
)

(func $json._msg_tuple_length_mismatch (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 21)))
  (memory.init $json_d_tuple_length_mismatch (local.get $ptr) (i32.const 0) (i32.const 21))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 21))
)

(func $json._msg_missing_field (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 13)))
  (memory.init $json_d_missing_field (local.get $ptr) (i32.const 0) (i32.const 13))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 13))
)

(func $json._msg_union_expected (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 14)))
  (memory.init $json_d_union_expected (local.get $ptr) (i32.const 0) (i32.const 14))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 14))
)

(func $json._msg_unsupported_schema_kind (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 23)))
  (memory.init $json_d_unsupported_schema_kind (local.get $ptr) (i32.const 0) (i32.const 23))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 23))
)

(func $json._msg_object_expected (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 15)))
  (memory.init $json_d_object_expected (local.get $ptr) (i32.const 0) (i32.const 15))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 15))
)

(func $json._str_empty (result anyref)
  (call $prelude.str_from_utf8 (i32.const 0) (i32.const 0))
)

(func $json._str_colon_space (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 2)))
  (memory.init $json_d_colon_space (local.get $ptr) (i32.const 0) (i32.const 2))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 2))
)

(func $json._str_dot (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 1)))
  (memory.init $json_d_dot (local.get $ptr) (i32.const 0) (i32.const 1))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 1))
)

(func $json._str_lbr (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 1)))
  (memory.init $json_d_lbr (local.get $ptr) (i32.const 0) (i32.const 1))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 1))
)

(func $json._str_rbr (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 1)))
  (memory.init $json_d_rbr (local.get $ptr) (i32.const 0) (i32.const 1))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 1))
)

(func $json._str_dollar (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 1)))
  (memory.init $json_d_dollar (local.get $ptr) (i32.const 0) (i32.const 1))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 1))
)

(func $json._k_kind (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 4)))
  (memory.init $json_d_kind (local.get $ptr) (i32.const 0) (i32.const 4))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 4))
)

(func $json._k_literal (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 7)))
  (memory.init $json_d_literal (local.get $ptr) (i32.const 0) (i32.const 7))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 7))
)

(func $json._k_value (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 5)))
  (memory.init $json_d_value (local.get $ptr) (i32.const 0) (i32.const 5))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 5))
)

(func $json._k_elem (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 4)))
  (memory.init $json_d_elem (local.get $ptr) (i32.const 0) (i32.const 4))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 4))
)

(func $json._k_tuple (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 5)))
  (memory.init $json_d_tuple (local.get $ptr) (i32.const 0) (i32.const 5))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 5))
)

(func $json._k_props (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 5)))
  (memory.init $json_d_props (local.get $ptr) (i32.const 0) (i32.const 5))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 5))
)

(func $json._k_name (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 4)))
  (memory.init $json_d_name (local.get $ptr) (i32.const 0) (i32.const 4))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 4))
)

(func $json._k_index (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 5)))
  (memory.init $json_d_index (local.get $ptr) (i32.const 0) (i32.const 5))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 5))
)

(func $json._k_union (result anyref)
  (local $ptr i32)
  (local.set $ptr (call $prelude._alloc (i32.const 5)))
  (memory.init $json_d_union (local.get $ptr) (i32.const 0) (i32.const 5))
  (call $prelude._new_string_owned (local.get $ptr) (i32.const 5))
)

(func $json._error_from_msg (param $msg anyref) (result anyref)
  (local $obj anyref)
  (local.set $obj (call $prelude.obj_new (i32.const 2)))
  (call $prelude.obj_set
    (local.get $obj)
    (call $json._str_type)
    (call $json._str_error))
  (call $prelude.obj_set
    (local.get $obj)
    (call $json._str_message)
    (local.get $msg))
  (local.get $obj)
)

(func $json._out_reset
  (global.set $json_out_len (i32.const 0))
)

(func $json._out_reserve (param $extra i32)
  (local $need i32)
  (local $cap i32)
  (local $new_ptr i32)

  (local.set $need
    (i32.add (global.get $json_out_len) (local.get $extra)))

  (if (i32.le_u (local.get $need) (global.get $json_out_cap))
    (then
      (return)
    )
  )

  (local.set $cap (global.get $json_out_cap))
  (if (i32.eqz (local.get $cap))
    (then
      (local.set $cap (i32.const 64))
    )
  )

  (block $cap_ok
    (loop $grow
      (br_if $cap_ok (i32.ge_u (local.get $cap) (local.get $need)))
      (local.set $cap (i32.shl (local.get $cap) (i32.const 1)))
      (br $grow)
    )
  )

  (local.set $new_ptr (call $prelude._alloc (local.get $cap)))
  (if (i32.gt_u (global.get $json_out_len) (i32.const 0))
    (then
      (memory.copy
        (local.get $new_ptr)
        (global.get $json_out_ptr)
        (global.get $json_out_len))
    )
  )

  (global.set $json_out_ptr (local.get $new_ptr))
  (global.set $json_out_cap (local.get $cap))
)

(func $json._append_byte (param $b i32)
  (call $json._out_reserve (i32.const 1))
  (i32.store8
    (i32.add (global.get $json_out_ptr) (global.get $json_out_len))
    (local.get $b))
  (global.set $json_out_len
    (i32.add (global.get $json_out_len) (i32.const 1)))
)

(func $json._append_bytes (param $ptr i32) (param $len i32)
  (if (i32.eqz (local.get $len))
    (then
      (return)
    )
  )
  (call $json._out_reserve (local.get $len))
  (memory.copy
    (i32.add (global.get $json_out_ptr) (global.get $json_out_len))
    (local.get $ptr)
    (local.get $len))
  (global.set $json_out_len
    (i32.add (global.get $json_out_len) (local.get $len)))
)

(func $json._append_string (param $s anyref)
  (call $json._append_bytes
    (call $prelude._string_ptr (local.get $s))
    (call $prelude._string_bytelen (local.get $s)))
)

(func $json._append_i64 (param $v i64)
  (call $json._append_string (call $prelude._i64_to_string (local.get $v)))
)

(func $json._append_true
  (call $json._append_byte (i32.const 116)) ;; t
  (call $json._append_byte (i32.const 114)) ;; r
  (call $json._append_byte (i32.const 117)) ;; u
  (call $json._append_byte (i32.const 101)) ;; e
)

(func $json._append_false
  (call $json._append_byte (i32.const 102)) ;; f
  (call $json._append_byte (i32.const 97))  ;; a
  (call $json._append_byte (i32.const 108)) ;; l
  (call $json._append_byte (i32.const 115)) ;; s
  (call $json._append_byte (i32.const 101)) ;; e
)

(func $json._append_null
  (call $json._append_byte (i32.const 110)) ;; n
  (call $json._append_byte (i32.const 117)) ;; u
  (call $json._append_byte (i32.const 108)) ;; l
  (call $json._append_byte (i32.const 108)) ;; l
)

(func $json._append_hex_nibble (param $n i32)
  (if (i32.lt_u (local.get $n) (i32.const 10))
    (then
      (call $json._append_byte (i32.add (i32.const 48) (local.get $n)))
    )
    (else
      (call $json._append_byte
        (i32.add (i32.const 97) (i32.sub (local.get $n) (i32.const 10))))
    )
  )
)

(func $json._write_quoted (param $value anyref)
  (local $ptr i32)
  (local $len i32)
  (local $i i32)
  (local $b i32)

  (call $json._append_byte (i32.const 34)) ;; "
  (local.set $ptr (call $prelude._string_ptr (local.get $value)))
  (local.set $len (call $prelude._string_bytelen (local.get $value)))
  (local.set $i (i32.const 0))

  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $b (i32.load8_u (i32.add (local.get $ptr) (local.get $i))))

      (if (i32.eq (local.get $b) (i32.const 34))
        (then
          (call $json._append_byte (i32.const 92)) ;; \
          (call $json._append_byte (i32.const 34)) ;; "
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 92))
        (then
          (call $json._append_byte (i32.const 92))
          (call $json._append_byte (i32.const 92))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 8))
        (then
          (call $json._append_byte (i32.const 92))
          (call $json._append_byte (i32.const 98)) ;; b
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 12))
        (then
          (call $json._append_byte (i32.const 92))
          (call $json._append_byte (i32.const 102)) ;; f
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 10))
        (then
          (call $json._append_byte (i32.const 92))
          (call $json._append_byte (i32.const 110)) ;; n
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 13))
        (then
          (call $json._append_byte (i32.const 92))
          (call $json._append_byte (i32.const 114)) ;; r
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $b) (i32.const 9))
        (then
          (call $json._append_byte (i32.const 92))
          (call $json._append_byte (i32.const 116)) ;; t
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.lt_u (local.get $b) (i32.const 32))
        (then
          (call $json._append_byte (i32.const 92))  ;; \
          (call $json._append_byte (i32.const 117)) ;; u
          (call $json._append_byte (i32.const 48))  ;; 0
          (call $json._append_byte (i32.const 48))  ;; 0
          (call $json._append_hex_nibble
            (i32.shr_u (local.get $b) (i32.const 4)))
          (call $json._append_hex_nibble
            (i32.and (local.get $b) (i32.const 15)))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )

      (call $json._append_byte (local.get $b))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )

  (call $json._append_byte (i32.const 34))
)

(func $json._write_f64_fixed (param $x f64) (param $max_frac i32)
  (local $ip i64)
  (local $frac f64)
  (local $digit i32)
  (local $i i32)
  (local $frac_start i32)
  (local $last_non_zero i32)

  (local.set $ip (i64.trunc_sat_f64_s (local.get $x)))
  (call $json._append_i64 (local.get $ip))

  (local.set $frac
    (f64.sub (local.get $x) (f64.convert_i64_s (local.get $ip))))
  (if (f64.lt (local.get $frac) (f64.const 0))
    (then
      (local.set $frac (f64.neg (local.get $frac)))
    )
  )
  (if (f64.lt (local.get $frac) (f64.const 1e-15))
    (then
      (return)
    )
  )

  (call $json._append_byte (i32.const 46)) ;; .
  (local.set $frac_start (global.get $json_out_len))
  (local.set $last_non_zero (i32.const -1))
  (local.set $i (i32.const 0))

  (block $digits_done
    (loop $digits
      (br_if $digits_done (i32.ge_u (local.get $i) (local.get $max_frac)))

      (local.set $frac (f64.mul (local.get $frac) (f64.const 10)))
      (local.set $digit (i32.trunc_sat_f64_u (local.get $frac)))
      (if (i32.gt_u (local.get $digit) (i32.const 9))
        (then
          (local.set $digit (i32.const 9))
        )
      )

      (call $json._append_byte (i32.add (i32.const 48) (local.get $digit)))
      (if (i32.ne (local.get $digit) (i32.const 0))
        (then
          (local.set $last_non_zero (local.get $i))
        )
      )

      (local.set $frac
        (f64.sub (local.get $frac) (f64.convert_i32_u (local.get $digit))))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))

      (br_if $digits_done (f64.lt (local.get $frac) (f64.const 1e-15)))
      (br $digits)
    )
  )

  (if (i32.eq (local.get $last_non_zero) (i32.const -1))
    (then
      ;; Remove '.' when the fractional part is effectively all zeros.
      (global.set $json_out_len (i32.sub (local.get $frac_start) (i32.const 1)))
      (return)
    )
  )

  ;; Trim trailing zeros.
  (global.set $json_out_len
    (i32.add
      (local.get $frac_start)
      (i32.add (local.get $last_non_zero) (i32.const 1))))
)

(func $json._write_f64_scientific (param $x f64)
  (local $m f64)
  (local $exp i32)

  (local.set $m (local.get $x))
  (local.set $exp (i32.const 0))

  (block $m_hi_done
    (loop $m_hi
      (br_if $m_hi_done (f64.lt (local.get $m) (f64.const 10)))
      (local.set $m (f64.div (local.get $m) (f64.const 10)))
      (local.set $exp (i32.add (local.get $exp) (i32.const 1)))
      (br $m_hi)
    )
  )

  (block $m_lo_done
    (loop $m_lo
      (br_if $m_lo_done (f64.ge (local.get $m) (f64.const 1)))
      (local.set $m (f64.mul (local.get $m) (f64.const 10)))
      (local.set $exp (i32.sub (local.get $exp) (i32.const 1)))
      (br $m_lo)
    )
  )

  (call $json._write_f64_fixed (local.get $m) (i32.const 14))
  (call $json._append_byte (i32.const 101)) ;; e

  (if (i32.lt_s (local.get $exp) (i32.const 0))
    (then
      (call $json._append_byte (i32.const 45)) ;; -
      (local.set $exp (i32.sub (i32.const 0) (local.get $exp)))
    )
    (else
      (call $json._append_byte (i32.const 43)) ;; +
    )
  )

  (call $json._append_i64 (i64.extend_i32_s (local.get $exp)))
)

(func $json._write_f64 (param $v f64)
  (local $x f64)
  (local $i i64)

  ;; Keep host behavior: NaN/Infinity are treated as invalid values.
  (if (f64.ne (local.get $v) (local.get $v))
    (then
      unreachable
    )
  )
  (if (f64.eq (local.get $v) (f64.const inf))
    (then
      unreachable
    )
  )
  (if (f64.eq (local.get $v) (f64.const -inf))
    (then
      unreachable
    )
  )

  (local.set $x (local.get $v))
  (if (f64.lt (local.get $x) (f64.const 0))
    (then
      (call $json._append_byte (i32.const 45)) ;; -
      (local.set $x (f64.neg (local.get $x)))
    )
  )

  (if (f64.eq (local.get $x) (f64.const 0))
    (then
      (call $json._append_byte (i32.const 48)) ;; 0
      (return)
    )
  )

  (if (f64.le (local.get $x) (f64.const 9.223372036854775e18))
    (then
      (local.set $i (i64.trunc_sat_f64_s (local.get $x)))
      (if (f64.eq (f64.convert_i64_s (local.get $i)) (local.get $x))
        (then
          (call $json._append_i64 (local.get $i))
          (return)
        )
      )
    )
  )

  ;; Approximate strconv.FormatFloat(..., 'g', -1, 64) thresholds.
  (if (i32.or
        (f64.ge (local.get $x) (f64.const 1e15))
        (f64.lt (local.get $x) (f64.const 1e-6)))
    (then
      (call $json._write_f64_scientific (local.get $x))
      (return)
    )
  )

  (call $json._write_f64_fixed (local.get $x) (i32.const 15))
)

(func $json._write_value (param $value anyref)
  (local $kind i32)
  (local.set $kind (call $prelude.val_kind (local.get $value)))

  (if (i32.eq (local.get $kind) (i32.const 0))
    (then
      (call $json._append_i64 (call $prelude.val_to_i64 (local.get $value)))
      (return)
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 1))
    (then
      (call $json._write_f64 (call $prelude.val_to_f64 (local.get $value)))
      (return)
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 2))
    (then
      (if (i32.eqz (call $prelude.val_to_bool (local.get $value)))
        (then
          (call $json._append_false)
        )
        (else
          (call $json._append_true)
        )
      )
      (return)
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 3))
    (then
      (call $json._write_quoted (local.get $value))
      (return)
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 5))
    (then
      (call $json._write_array (local.get $value))
      (return)
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 4))
    (then
      (call $json._write_object (local.get $value))
      (return)
    )
  )
  (if (i32.eq (local.get $kind) (i32.const 6))
    (then
      (call $json._append_null)
      (return)
    )
  )

  ;; undefined/unknown values are emitted as null.
  (call $json._append_null)
)

(func $json._write_array (param $arr anyref)
  (local $len i32)
  (local $i i32)

  (call $json._append_byte (i32.const 91)) ;; [
  (local.set $len (call $prelude.arr_len (local.get $arr)))
  (local.set $i (i32.const 0))

  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $len)))

      (if (i32.gt_u (local.get $i) (i32.const 0))
        (then
          (call $json._append_byte (i32.const 44)) ;; ,
        )
      )

      (call $json._write_value
        (call $prelude.arr_get (local.get $arr) (local.get $i)))

      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )

  (call $json._append_byte (i32.const 93)) ;; ]
)

(func $json._write_object (param $obj anyref)
  (local $keys anyref)
  (local $len i32)
  (local $i i32)
  (local $wrote i32)
  (local $key anyref)
  (local $val anyref)

  (call $json._append_byte (i32.const 123)) ;; {
  (local.set $keys (call $prelude.obj_keys (local.get $obj)))
  (local.set $len (call $prelude.arr_len (local.get $keys)))
  (local.set $i (i32.const 0))
  (local.set $wrote (i32.const 0))

  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $key (call $prelude.arr_get (local.get $keys) (local.get $i)))
      (local.set $val (call $prelude.obj_get (local.get $obj) (local.get $key)))

      ;; Omit object properties whose value is undefined.
      (if (i32.eq (call $prelude.val_kind (local.get $val)) (i32.const 7))
        (then
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $loop)
        )
      )

      (if (i32.gt_u (local.get $wrote) (i32.const 0))
        (then
          (call $json._append_byte (i32.const 44)) ;; ,
        )
      )
      (local.set $wrote (i32.add (local.get $wrote) (i32.const 1)))

      (call $json._write_quoted (local.get $key))
      (call $json._append_byte (i32.const 58)) ;; :
      (call $json._write_value (local.get $val))

      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )

  (call $json._append_byte (i32.const 125)) ;; }
)

(func $json.stringify (param $value anyref) (result anyref)
  (call $json._out_reset)
  (call $json._write_value (local.get $value))
  (call $prelude.str_from_utf8
    (global.get $json_out_ptr)
    (global.get $json_out_len))
)

(func $json._parse_reset (param $ptr i32) (param $len i32)
  (global.set $json_parse_ptr (local.get $ptr))
  (global.set $json_parse_end (i32.add (local.get $ptr) (local.get $len)))
  (global.set $json_parse_err (i32.const 0))
  (global.set $json_parse_err_msg (ref.null any))
)

(func $json._parse_set_error (param $msg anyref)
  (if (i32.eqz (global.get $json_parse_err))
    (then
      (global.set $json_parse_err (i32.const 1))
      (global.set $json_parse_err_msg (local.get $msg))
    )
  )
)

(func $json._parse_fail_invalid
  (call $json._parse_set_error (call $json._msg_invalid_json))
)

(func $json._is_digit (param $c i32) (result i32)
  (i32.and
    (i32.ge_u (local.get $c) (i32.const 48))
    (i32.le_u (local.get $c) (i32.const 57)))
)

(func $json._hex_nibble (param $c i32) (result i32)
  (if (i32.and
        (i32.ge_u (local.get $c) (i32.const 48))
        (i32.le_u (local.get $c) (i32.const 57)))
    (then
      (return (i32.sub (local.get $c) (i32.const 48)))
    )
  )
  (if (i32.and
        (i32.ge_u (local.get $c) (i32.const 65))
        (i32.le_u (local.get $c) (i32.const 70)))
    (then
      (return (i32.sub (local.get $c) (i32.const 55)))
    )
  )
  (if (i32.and
        (i32.ge_u (local.get $c) (i32.const 97))
        (i32.le_u (local.get $c) (i32.const 102)))
    (then
      (return (i32.sub (local.get $c) (i32.const 87)))
    )
  )
  (i32.const -1)
)

(func $json._parse_hex4 (result i32)
  (local $n i32)
  (local $i i32)
  (local $c i32)
  (local $d i32)

  (local.set $n (i32.const 0))
  (local.set $i (i32.const 0))

  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (i32.const 4)))
      (if (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
        (then
          (return (i32.const -1))
        )
      )

      (local.set $c (i32.load8_u (global.get $json_parse_ptr)))
      (global.set $json_parse_ptr
        (i32.add (global.get $json_parse_ptr) (i32.const 1)))
      (local.set $d (call $json._hex_nibble (local.get $c)))
      (if (i32.lt_s (local.get $d) (i32.const 0))
        (then
          (return (i32.const -1))
        )
      )

      (local.set $n
        (i32.add
          (i32.shl (local.get $n) (i32.const 4))
          (local.get $d)))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )

  (local.get $n)
)

(func $json._append_utf8 (param $buf i32) (param $len i32) (param $cp i32) (result i32)
  (if (i32.or
        (i32.lt_s (local.get $cp) (i32.const 0))
        (i32.gt_u (local.get $cp) (i32.const 1114111)))
    (then
      (return (i32.const -1))
    )
  )

  (if (i32.le_u (local.get $cp) (i32.const 127))
    (then
      (i32.store8
        (i32.add (local.get $buf) (local.get $len))
        (local.get $cp))
      (return (i32.add (local.get $len) (i32.const 1)))
    )
  )

  (if (i32.le_u (local.get $cp) (i32.const 2047))
    (then
      (i32.store8
        (i32.add (local.get $buf) (local.get $len))
        (i32.or
          (i32.const 192)
          (i32.shr_u (local.get $cp) (i32.const 6))))
      (i32.store8
        (i32.add (local.get $buf) (i32.add (local.get $len) (i32.const 1)))
        (i32.or
          (i32.const 128)
          (i32.and (local.get $cp) (i32.const 63))))
      (return (i32.add (local.get $len) (i32.const 2)))
    )
  )

  (if (i32.le_u (local.get $cp) (i32.const 65535))
    (then
      (i32.store8
        (i32.add (local.get $buf) (local.get $len))
        (i32.or
          (i32.const 224)
          (i32.shr_u (local.get $cp) (i32.const 12))))
      (i32.store8
        (i32.add (local.get $buf) (i32.add (local.get $len) (i32.const 1)))
        (i32.or
          (i32.const 128)
          (i32.and
            (i32.shr_u (local.get $cp) (i32.const 6))
            (i32.const 63))))
      (i32.store8
        (i32.add (local.get $buf) (i32.add (local.get $len) (i32.const 2)))
        (i32.or
          (i32.const 128)
          (i32.and (local.get $cp) (i32.const 63))))
      (return (i32.add (local.get $len) (i32.const 3)))
    )
  )

  (i32.store8
    (i32.add (local.get $buf) (local.get $len))
    (i32.or
      (i32.const 240)
      (i32.shr_u (local.get $cp) (i32.const 18))))
  (i32.store8
    (i32.add (local.get $buf) (i32.add (local.get $len) (i32.const 1)))
    (i32.or
      (i32.const 128)
      (i32.and
        (i32.shr_u (local.get $cp) (i32.const 12))
        (i32.const 63))))
  (i32.store8
    (i32.add (local.get $buf) (i32.add (local.get $len) (i32.const 2)))
    (i32.or
      (i32.const 128)
      (i32.and
        (i32.shr_u (local.get $cp) (i32.const 6))
        (i32.const 63))))
  (i32.store8
    (i32.add (local.get $buf) (i32.add (local.get $len) (i32.const 3)))
    (i32.or
      (i32.const 128)
      (i32.and (local.get $cp) (i32.const 63))))
  (i32.add (local.get $len) (i32.const 4))
)

(func $json._parse_skip_ws
  (local $c i32)
  (block $done
    (loop $loop
      (br_if $done
        (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end)))
      (local.set $c (i32.load8_u (global.get $json_parse_ptr)))
      (if (i32.or
            (i32.or
              (i32.eq (local.get $c) (i32.const 32))
              (i32.eq (local.get $c) (i32.const 9)))
            (i32.or
              (i32.eq (local.get $c) (i32.const 10))
              (i32.eq (local.get $c) (i32.const 13))))
        (then
          (global.set $json_parse_ptr
            (i32.add (global.get $json_parse_ptr) (i32.const 1)))
          (br $loop)
        )
      )
      (br $done)
    )
  )
)

(func $json._parse_f64_token (param $start i32) (param $len i32) (result f64)
  (local $idx i32)
  (local $end i32)
  (local $neg i32)
  (local $digit i32)
  (local $c i32)
  (local $val f64)
  (local $factor f64)
  (local $exp i32)
  (local $exp_neg i32)
  (local $k i32)

  (local.set $idx (local.get $start))
  (local.set $end (i32.add (local.get $start) (local.get $len)))
  (local.set $neg (i32.const 0))
  (local.set $val (f64.const 0))
  (local.set $exp (i32.const 0))
  (local.set $exp_neg (i32.const 0))

  (if (i32.and
        (i32.lt_u (local.get $idx) (local.get $end))
        (i32.eq
          (i32.load8_u (local.get $idx))
          (i32.const 45)))
    (then
      (local.set $neg (i32.const 1))
      (local.set $idx (i32.add (local.get $idx) (i32.const 1)))
    )
  )

  (block $int_done
    (loop $int_loop
      (br_if $int_done (i32.ge_u (local.get $idx) (local.get $end)))
      (local.set $c (i32.load8_u (local.get $idx)))
      (br_if $int_done (i32.eqz (call $json._is_digit (local.get $c))))
      (local.set $digit (i32.sub (local.get $c) (i32.const 48)))
      (local.set $val
        (f64.add
          (f64.mul (local.get $val) (f64.const 10))
          (f64.convert_i32_u (local.get $digit))))
      (local.set $idx (i32.add (local.get $idx) (i32.const 1)))
      (br $int_loop)
    )
  )

  (if (i32.and
        (i32.lt_u (local.get $idx) (local.get $end))
        (i32.eq
          (i32.load8_u (local.get $idx))
          (i32.const 46)))
    (then
      (local.set $idx (i32.add (local.get $idx) (i32.const 1)))
      (local.set $factor (f64.const 0.1))
      (block $frac_done
        (loop $frac_loop
          (br_if $frac_done (i32.ge_u (local.get $idx) (local.get $end)))
          (local.set $c (i32.load8_u (local.get $idx)))
          (br_if $frac_done (i32.eqz (call $json._is_digit (local.get $c))))
          (local.set $digit (i32.sub (local.get $c) (i32.const 48)))
          (local.set $val
            (f64.add
              (local.get $val)
              (f64.mul
                (f64.convert_i32_u (local.get $digit))
                (local.get $factor))))
          (local.set $factor
            (f64.mul (local.get $factor) (f64.const 0.1)))
          (local.set $idx (i32.add (local.get $idx) (i32.const 1)))
          (br $frac_loop)
        )
      )
    )
  )

  (if (i32.and
        (i32.lt_u (local.get $idx) (local.get $end))
        (i32.or
          (i32.eq
            (i32.load8_u (local.get $idx))
            (i32.const 101))
          (i32.eq
            (i32.load8_u (local.get $idx))
            (i32.const 69))))
    (then
      (local.set $idx (i32.add (local.get $idx) (i32.const 1)))
      (if (i32.and
            (i32.lt_u (local.get $idx) (local.get $end))
            (i32.eq
              (i32.load8_u (local.get $idx))
              (i32.const 43)))
        (then
          (local.set $idx (i32.add (local.get $idx) (i32.const 1)))
        )
      )
      (if (i32.and
            (i32.lt_u (local.get $idx) (local.get $end))
            (i32.eq
              (i32.load8_u (local.get $idx))
              (i32.const 45)))
        (then
          (local.set $exp_neg (i32.const 1))
          (local.set $idx (i32.add (local.get $idx) (i32.const 1)))
        )
      )
      (local.set $exp (i32.const 0))
      (block $exp_done
        (loop $exp_loop
          (br_if $exp_done (i32.ge_u (local.get $idx) (local.get $end)))
          (local.set $c (i32.load8_u (local.get $idx)))
          (br_if $exp_done (i32.eqz (call $json._is_digit (local.get $c))))
          (local.set $exp
            (i32.add
              (i32.mul (local.get $exp) (i32.const 10))
              (i32.sub (local.get $c) (i32.const 48))))
          (local.set $idx (i32.add (local.get $idx) (i32.const 1)))
          (br $exp_loop)
        )
      )
      (if (local.get $exp_neg)
        (then
          (local.set $exp (i32.sub (i32.const 0) (local.get $exp)))
        )
      )
    )
  )

  (if (i32.gt_s (local.get $exp) (i32.const 308))
    (then
      (local.set $val (f64.const inf))
    )
    (else
      (if (i32.lt_s (local.get $exp) (i32.const -324))
        (then
          (local.set $val (f64.const 0))
        )
        (else
          (if (i32.gt_s (local.get $exp) (i32.const 0))
            (then
              (local.set $k (i32.const 0))
              (block $pow_pos_done
                (loop $pow_pos
                  (br_if $pow_pos_done
                    (i32.ge_u (local.get $k) (local.get $exp)))
                  (local.set $val
                    (f64.mul (local.get $val) (f64.const 10)))
                  (local.set $k (i32.add (local.get $k) (i32.const 1)))
                  (br $pow_pos)
                )
              )
            )
            (else
              (if (i32.lt_s (local.get $exp) (i32.const 0))
                (then
                  (local.set $k (i32.const 0))
                  (block $pow_neg_done
                    (loop $pow_neg
                      (br_if $pow_neg_done
                        (i32.ge_u
                          (local.get $k)
                          (i32.sub (i32.const 0) (local.get $exp))))
                      (local.set $val
                        (f64.div (local.get $val) (f64.const 10)))
                      (local.set $k (i32.add (local.get $k) (i32.const 1)))
                      (br $pow_neg)
                    )
                  )
                )
              )
            )
          )
        )
      )
    )
  )

  (if (local.get $neg)
    (then
      (local.set $val (f64.neg (local.get $val)))
    )
  )
  (local.get $val)
)

(func $json._str_cmp (param $a anyref) (param $b anyref) (result i32)
  (local $aptr i32)
  (local $bptr i32)
  (local $alen i32)
  (local $blen i32)
  (local $min i32)
  (local $i i32)
  (local $ac i32)
  (local $bc i32)

  (local.set $aptr (call $prelude._string_ptr (local.get $a)))
  (local.set $bptr (call $prelude._string_ptr (local.get $b)))
  (local.set $alen (call $prelude._string_bytelen (local.get $a)))
  (local.set $blen (call $prelude._string_bytelen (local.get $b)))
  (local.set $min (local.get $alen))
  (if (i32.gt_u (local.get $min) (local.get $blen))
    (then
      (local.set $min (local.get $blen))
    )
  )
  (local.set $i (i32.const 0))

  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $min)))
      (local.set $ac (i32.load8_u (i32.add (local.get $aptr) (local.get $i))))
      (local.set $bc (i32.load8_u (i32.add (local.get $bptr) (local.get $i))))
      (if (i32.lt_u (local.get $ac) (local.get $bc))
        (then
          (return (i32.const -1))
        )
      )
      (if (i32.gt_u (local.get $ac) (local.get $bc))
        (then
          (return (i32.const 1))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )

  (if (i32.lt_u (local.get $alen) (local.get $blen))
    (then
      (return (i32.const -1))
    )
  )
  (if (i32.gt_u (local.get $alen) (local.get $blen))
    (then
      (return (i32.const 1))
    )
  )
  (i32.const 0)
)

(func $json._obj_has_key (param $obj anyref) (param $key anyref) (result i32)
  (local $keys anyref)
  (local $len i32)
  (local $i i32)
  (local $k anyref)

  (if (i32.ne (call $prelude.val_kind (local.get $obj)) (i32.const 4))
    (then
      (return (i32.const 0))
    )
  )

  (local.set $keys (call $prelude.obj_keys (local.get $obj)))
  (local.set $len (call $prelude.arr_len (local.get $keys)))
  (local.set $i (i32.const 0))

  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $k (call $prelude.arr_get (local.get $keys) (local.get $i)))
      (if (call $prelude.str_eq (local.get $k) (local.get $key))
        (then
          (return (i32.const 1))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )
  (i32.const 0)
)

(func $json._arr_find_string (param $arr anyref) (param $count i32) (param $key anyref) (result i32)
  (local $i i32)
  (local $k anyref)
  (local.set $i (i32.const 0))
  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $count)))
      (local.set $k (call $prelude.arr_get (local.get $arr) (local.get $i)))
      (if (call $prelude.str_eq (local.get $k) (local.get $key))
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

(func $json._decode_reset
  (global.set $json_decode_err (i32.const 0))
  (global.set $json_decode_err_msg (ref.null any))
)

(func $json._decode_set_error (param $msg anyref)
  (if (i32.eqz (global.get $json_decode_err))
    (then
      (global.set $json_decode_err (i32.const 1))
      (global.set $json_decode_err_msg (local.get $msg))
    )
  )
)

(func $json._decode_force_error (param $msg anyref)
  (global.set $json_decode_err (i32.const 1))
  (global.set $json_decode_err_msg (local.get $msg))
)

(func $json._decode_error_at (param $path anyref) (param $suffix anyref)
  (local $head anyref)
  (local.set $head
    (call $prelude.str_concat
      (local.get $path)
      (call $json._str_colon_space)))
  (call $json._decode_set_error
    (call $prelude.str_concat (local.get $head) (local.get $suffix)))
)

(func $json._decode_invalid_schema_at (param $path anyref)
  (call $json._decode_error_at (local.get $path) (call $json._msg_invalid_schema))
)

(func $json._path_field (param $path anyref) (param $name anyref) (result anyref)
  (local $head anyref)
  (local.set $head
    (call $prelude.str_concat
      (local.get $path)
      (call $json._str_dot)))
  (call $prelude.str_concat (local.get $head) (local.get $name))
)

(func $json._path_index (param $path anyref) (param $index i32) (result anyref)
  (local $head anyref)
  (local $mid anyref)
  (local $idx anyref)
  (local.set $head
    (call $prelude.str_concat
      (local.get $path)
      (call $json._str_lbr)))
  (local.set $idx
    (call $prelude._i64_to_string
      (i64.extend_i32_u (local.get $index))))
  (local.set $mid (call $prelude.str_concat (local.get $head) (local.get $idx)))
  (call $prelude.str_concat (local.get $mid) (call $json._str_rbr))
)

(func $json._is_error_object (param $value anyref) (result i32)
  (local $t anyref)
  (if (i32.ne (call $prelude.val_kind (local.get $value)) (i32.const 4))
    (then
      (return (i32.const 0))
    )
  )
  (if (i32.eqz (call $json._obj_has_key (local.get $value) (call $json._str_type)))
    (then
      (return (i32.const 0))
    )
  )
  (local.set $t (call $prelude.obj_get (local.get $value) (call $json._str_type)))
  (if (i32.ne (call $prelude.val_kind (local.get $t)) (i32.const 3))
    (then
      (return (i32.const 0))
    )
  )
  (if (call $prelude.str_eq (local.get $t) (call $json._str_error))
    (then
      (return (i32.const 1))
    )
  )
  (i32.const 0)
)

(func $json._schema_kind_code (param $kind anyref) (result i32)
  (local $ptr i32)
  (local $len i32)
  (local $b0 i32)
  (local $b1 i32)
  (local $b2 i32)
  (local $b3 i32)
  (local $b4 i32)
  (local $b5 i32)
  (local $b6 i32)
  (local $b7 i32)
  (local $b8 i32)

  (if (i32.ne (call $prelude.val_kind (local.get $kind)) (i32.const 3))
    (then
      (return (i32.const 0))
    )
  )

  (local.set $ptr (call $prelude._string_ptr (local.get $kind)))
  (local.set $len (call $prelude._string_bytelen (local.get $kind)))
  (if (i32.eqz (local.get $len))
    (then
      (return (i32.const 0))
    )
  )

  (local.set $b0 (i32.load8_u (local.get $ptr)))
  (if (i32.ge_u (local.get $len) (i32.const 2))
    (then (local.set $b1 (i32.load8_u (i32.add (local.get $ptr) (i32.const 1)))))
  )
  (if (i32.ge_u (local.get $len) (i32.const 3))
    (then (local.set $b2 (i32.load8_u (i32.add (local.get $ptr) (i32.const 2)))))
  )
  (if (i32.ge_u (local.get $len) (i32.const 4))
    (then (local.set $b3 (i32.load8_u (i32.add (local.get $ptr) (i32.const 3)))))
  )
  (if (i32.ge_u (local.get $len) (i32.const 5))
    (then (local.set $b4 (i32.load8_u (i32.add (local.get $ptr) (i32.const 4)))))
  )
  (if (i32.ge_u (local.get $len) (i32.const 6))
    (then (local.set $b5 (i32.load8_u (i32.add (local.get $ptr) (i32.const 5)))))
  )
  (if (i32.ge_u (local.get $len) (i32.const 7))
    (then (local.set $b6 (i32.load8_u (i32.add (local.get $ptr) (i32.const 6)))))
  )
  (if (i32.ge_u (local.get $len) (i32.const 8))
    (then (local.set $b7 (i32.load8_u (i32.add (local.get $ptr) (i32.const 7)))))
  )
  (if (i32.ge_u (local.get $len) (i32.const 9))
    (then (local.set $b8 (i32.load8_u (i32.add (local.get $ptr) (i32.const 8)))))
  )

  (if (i32.eq (local.get $len) (i32.const 4))
    (then
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 106))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 115))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 111))
                (i32.eq (local.get $b3) (i32.const 110)))))
        (then
          (return (i32.const 1))
        )
      )
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 110))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 117))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 108))
                (i32.eq (local.get $b3) (i32.const 108)))))
        (then
          (return (i32.const 3))
        )
      )
    )
  )

  (if (i32.eq (local.get $len) (i32.const 5))
    (then
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 97))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 114))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 114))
                (i32.and
                  (i32.eq (local.get $b3) (i32.const 97))
                  (i32.eq (local.get $b4) (i32.const 121))))))
        (then
          (return (i32.const 8))
        )
      )
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 116))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 117))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 112))
                (i32.and
                  (i32.eq (local.get $b3) (i32.const 108))
                  (i32.eq (local.get $b4) (i32.const 101))))))
        (then
          (return (i32.const 9))
        )
      )
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 117))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 110))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 105))
                (i32.and
                  (i32.eq (local.get $b3) (i32.const 111))
                  (i32.eq (local.get $b4) (i32.const 110))))))
        (then
          (return (i32.const 11))
        )
      )
    )
  )

  (if (i32.eq (local.get $len) (i32.const 6))
    (then
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 115))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 116))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 114))
                (i32.and
                  (i32.eq (local.get $b3) (i32.const 105))
                  (i32.and
                    (i32.eq (local.get $b4) (i32.const 110))
                    (i32.eq (local.get $b5) (i32.const 103)))))))
        (then
          (return (i32.const 4))
        )
      )
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 110))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 117))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 109))
                (i32.and
                  (i32.eq (local.get $b3) (i32.const 98))
                  (i32.and
                    (i32.eq (local.get $b4) (i32.const 101))
                    (i32.eq (local.get $b5) (i32.const 114)))))))
        (then
          (return (i32.const 7))
        )
      )
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 111))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 98))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 106))
                (i32.and
                  (i32.eq (local.get $b3) (i32.const 101))
                  (i32.and
                    (i32.eq (local.get $b4) (i32.const 99))
                    (i32.eq (local.get $b5) (i32.const 116)))))))
        (then
          (return (i32.const 10))
        )
      )
    )
  )

  (if (i32.eq (local.get $len) (i32.const 7))
    (then
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 98))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 111))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 111))
                (i32.and
                  (i32.eq (local.get $b3) (i32.const 108))
                  (i32.and
                    (i32.eq (local.get $b4) (i32.const 101))
                    (i32.and
                      (i32.eq (local.get $b5) (i32.const 97))
                      (i32.eq (local.get $b6) (i32.const 110))))))))
        (then
          (return (i32.const 5))
        )
      )
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 105))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 110))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 116))
                (i32.and
                  (i32.eq (local.get $b3) (i32.const 101))
                  (i32.and
                    (i32.eq (local.get $b4) (i32.const 103))
                    (i32.and
                      (i32.eq (local.get $b5) (i32.const 101))
                      (i32.eq (local.get $b6) (i32.const 114))))))))
        (then
          (return (i32.const 6))
        )
      )
    )
  )

  (if (i32.eq (local.get $len) (i32.const 9))
    (then
      (if (i32.and
            (i32.eq (local.get $b0) (i32.const 117))
            (i32.and
              (i32.eq (local.get $b1) (i32.const 110))
              (i32.and
                (i32.eq (local.get $b2) (i32.const 100))
                (i32.and
                  (i32.eq (local.get $b3) (i32.const 101))
                  (i32.and
                    (i32.eq (local.get $b4) (i32.const 102))
                    (i32.and
                      (i32.eq (local.get $b5) (i32.const 105))
                      (i32.and
                        (i32.eq (local.get $b6) (i32.const 110))
                        (i32.and
                          (i32.eq (local.get $b7) (i32.const 101))
                          (i32.eq (local.get $b8) (i32.const 100))))))))))
        (then
          (return (i32.const 2))
        )
      )
    )
  )

  (i32.const 0)
)

(func $json._literal_as_string (param $lit anyref) (result anyref)
  (local $v anyref)
  (if (i32.eqz (call $json._obj_has_key (local.get $lit) (call $json._k_value)))
    (then
      (return (call $json._str_empty))
    )
  )
  (local.set $v (call $prelude.obj_get (local.get $lit) (call $json._k_value)))
  (if (i32.eq (call $prelude.val_kind (local.get $v)) (i32.const 3))
    (then
      (return (local.get $v))
    )
  )
  (call $json._str_empty)
)

(func $json._literal_as_bool (param $lit anyref) (result i32)
  (local $v anyref)
  (if (i32.eqz (call $json._obj_has_key (local.get $lit) (call $json._k_value)))
    (then
      (return (i32.const 0))
    )
  )
  (local.set $v (call $prelude.obj_get (local.get $lit) (call $json._k_value)))
  (if (i32.eq (call $prelude.val_kind (local.get $v)) (i32.const 2))
    (then
      (return (call $prelude.val_to_bool (local.get $v)))
    )
  )
  (i32.const 0)
)

(func $json._literal_as_f64 (param $lit anyref) (result f64)
  (local $v anyref)
  (local $k i32)
  (if (i32.eqz (call $json._obj_has_key (local.get $lit) (call $json._k_value)))
    (then
      (return (f64.const 0))
    )
  )
  (local.set $v (call $prelude.obj_get (local.get $lit) (call $json._k_value)))
  (local.set $k (call $prelude.val_kind (local.get $v)))
  (if (i32.eq (local.get $k) (i32.const 1))
    (then
      (return (call $prelude.val_to_f64 (local.get $v)))
    )
  )
  (if (i32.eq (local.get $k) (i32.const 0))
    (then
      (return (f64.convert_i64_s (call $prelude.val_to_i64 (local.get $v))))
    )
  )
  (f64.const 0)
)

(func $json._schema_allows_undefined (param $schema anyref) (result i32)
  (local $kind anyref)
  (local $code i32)
  (local $union anyref)
  (local $len i32)
  (local $i i32)
  (local $member anyref)

  (if (i32.ne (call $prelude.val_kind (local.get $schema)) (i32.const 4))
    (then
      (return (i32.const 0))
    )
  )
  (if (i32.eqz (call $json._obj_has_key (local.get $schema) (call $json._k_kind)))
    (then
      (return (i32.const 0))
    )
  )
  (local.set $kind (call $prelude.obj_get (local.get $schema) (call $json._k_kind)))
  (local.set $code (call $json._schema_kind_code (local.get $kind)))
  (if (i32.eq (local.get $code) (i32.const 2))
    (then
      (return (i32.const 1))
    )
  )
  (if (i32.ne (local.get $code) (i32.const 11))
    (then
      (return (i32.const 0))
    )
  )
  (if (i32.eqz (call $json._obj_has_key (local.get $schema) (call $json._k_union)))
    (then
      (return (i32.const 0))
    )
  )
  (local.set $union (call $prelude.obj_get (local.get $schema) (call $json._k_union)))
  (if (i32.ne (call $prelude.val_kind (local.get $union)) (i32.const 5))
    (then
      (return (i32.const 0))
    )
  )

  (local.set $len (call $prelude.arr_len (local.get $union)))
  (local.set $i (i32.const 0))
  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $member (call $prelude.arr_get (local.get $union) (local.get $i)))
      (if (call $json._schema_allows_undefined (local.get $member))
        (then
          (return (i32.const 1))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )
  (i32.const 0)
)

(func $json._parse_string (result anyref)
  (local $raw_cap i32)
  (local $out_ptr i32)
  (local $out_len i32)
  (local $b i32)
  (local $esc i32)
  (local $cp i32)
  (local $cp2 i32)
  (local $new_len i32)

  (if (global.get $json_parse_err)
    (then
      (return (call $prelude.val_undefined))
    )
  )
  (if (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )
  (if (i32.ne
        (i32.load8_u (global.get $json_parse_ptr))
        (i32.const 34))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )

  (global.set $json_parse_ptr
    (i32.add (global.get $json_parse_ptr) (i32.const 1)))

  (local.set $raw_cap
    (i32.add
      (i32.sub (global.get $json_parse_end) (global.get $json_parse_ptr))
      (i32.const 1)))
  (if (i32.lt_s (local.get $raw_cap) (i32.const 1))
    (then
      (local.set $raw_cap (i32.const 1))
    )
  )
  (local.set $out_ptr (call $prelude._alloc (local.get $raw_cap)))
  (local.set $out_len (i32.const 0))

  (block $done
    (loop $loop
      (if (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )
      (local.set $b (i32.load8_u (global.get $json_parse_ptr)))
      (global.set $json_parse_ptr
        (i32.add (global.get $json_parse_ptr) (i32.const 1)))

      (if (i32.eq (local.get $b) (i32.const 34))
        (then
          (return (call $prelude.str_from_utf8 (local.get $out_ptr) (local.get $out_len)))
        )
      )
      (if (i32.lt_u (local.get $b) (i32.const 32))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )

      (if (i32.ne (local.get $b) (i32.const 92))
        (then
          (i32.store8
            (i32.add (local.get $out_ptr) (local.get $out_len))
            (local.get $b))
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 1)))
          (br $loop)
        )
      )

      (if (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )
      (local.set $esc (i32.load8_u (global.get $json_parse_ptr)))
      (global.set $json_parse_ptr
        (i32.add (global.get $json_parse_ptr) (i32.const 1)))

      (if (i32.or
            (i32.or
              (i32.eq (local.get $esc) (i32.const 34))
              (i32.eq (local.get $esc) (i32.const 47)))
            (i32.eq (local.get $esc) (i32.const 92)))
        (then
          (i32.store8
            (i32.add (local.get $out_ptr) (local.get $out_len))
            (local.get $esc))
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $esc) (i32.const 98))
        (then
          (i32.store8
            (i32.add (local.get $out_ptr) (local.get $out_len))
            (i32.const 8))
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $esc) (i32.const 102))
        (then
          (i32.store8
            (i32.add (local.get $out_ptr) (local.get $out_len))
            (i32.const 12))
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $esc) (i32.const 110))
        (then
          (i32.store8
            (i32.add (local.get $out_ptr) (local.get $out_len))
            (i32.const 10))
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $esc) (i32.const 114))
        (then
          (i32.store8
            (i32.add (local.get $out_ptr) (local.get $out_len))
            (i32.const 13))
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 1)))
          (br $loop)
        )
      )
      (if (i32.eq (local.get $esc) (i32.const 116))
        (then
          (i32.store8
            (i32.add (local.get $out_ptr) (local.get $out_len))
            (i32.const 9))
          (local.set $out_len (i32.add (local.get $out_len) (i32.const 1)))
          (br $loop)
        )
      )

      (if (i32.eq (local.get $esc) (i32.const 117))
        (then
          (local.set $cp (call $json._parse_hex4))
          (if (i32.lt_s (local.get $cp) (i32.const 0))
            (then
              (call $json._parse_fail_invalid)
              (return (call $prelude.val_undefined))
            )
          )

          (if (i32.and
                (i32.ge_u (local.get $cp) (i32.const 55296))
                (i32.le_u (local.get $cp) (i32.const 56319)))
            (then
              (if (i32.ge_u
                    (i32.add (global.get $json_parse_ptr) (i32.const 1))
                    (global.get $json_parse_end))
                (then
                  (call $json._parse_fail_invalid)
                  (return (call $prelude.val_undefined))
                )
              )
              (if (i32.or
                    (i32.ne
                      (i32.load8_u (global.get $json_parse_ptr))
                      (i32.const 92))
                    (i32.ne
                      (i32.load8_u
                        (i32.add (global.get $json_parse_ptr) (i32.const 1)))
                      (i32.const 117)))
                (then
                  (call $json._parse_fail_invalid)
                  (return (call $prelude.val_undefined))
                )
              )
              (global.set $json_parse_ptr
                (i32.add (global.get $json_parse_ptr) (i32.const 2)))
              (local.set $cp2 (call $json._parse_hex4))
              (if (i32.or
                    (i32.lt_u (local.get $cp2) (i32.const 56320))
                    (i32.gt_u (local.get $cp2) (i32.const 57343)))
                (then
                  (call $json._parse_fail_invalid)
                  (return (call $prelude.val_undefined))
                )
              )
              (local.set $cp
                (i32.add
                  (i32.const 65536)
                  (i32.add
                    (i32.shl
                      (i32.sub (local.get $cp) (i32.const 55296))
                      (i32.const 10))
                    (i32.sub (local.get $cp2) (i32.const 56320)))))
            )
            (else
              (if (i32.and
                    (i32.ge_u (local.get $cp) (i32.const 56320))
                    (i32.le_u (local.get $cp) (i32.const 57343)))
                (then
                  (call $json._parse_fail_invalid)
                  (return (call $prelude.val_undefined))
                )
              )
            )
          )

          (local.set $new_len
            (call $json._append_utf8 (local.get $out_ptr) (local.get $out_len) (local.get $cp)))
          (if (i32.lt_s (local.get $new_len) (i32.const 0))
            (then
              (call $json._parse_fail_invalid)
              (return (call $prelude.val_undefined))
            )
          )
          (local.set $out_len (local.get $new_len))
          (br $loop)
        )
      )

      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )

  (call $json._parse_fail_invalid)
  (call $prelude.val_undefined)
)

(func $json._parse_true (result anyref)
  (local $p i32)
  (if (global.get $json_parse_err)
    (then
      (return (call $prelude.val_undefined))
    )
  )
  (local.set $p (global.get $json_parse_ptr))
  (if (i32.gt_u
        (i32.add (local.get $p) (i32.const 4))
        (global.get $json_parse_end))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )
  (if (i32.or
        (i32.or
          (i32.ne (i32.load8_u (local.get $p)) (i32.const 116))
          (i32.ne (i32.load8_u (i32.add (local.get $p) (i32.const 1))) (i32.const 114)))
        (i32.or
          (i32.ne (i32.load8_u (i32.add (local.get $p) (i32.const 2))) (i32.const 117))
          (i32.ne (i32.load8_u (i32.add (local.get $p) (i32.const 3))) (i32.const 101))))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )
  (global.set $json_parse_ptr (i32.add (local.get $p) (i32.const 4)))
  (call $prelude.val_from_bool (i32.const 1))
)

(func $json._parse_false (result anyref)
  (local $p i32)
  (if (global.get $json_parse_err)
    (then
      (return (call $prelude.val_undefined))
    )
  )
  (local.set $p (global.get $json_parse_ptr))
  (if (i32.gt_u
        (i32.add (local.get $p) (i32.const 5))
        (global.get $json_parse_end))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )
  (if (i32.or
        (i32.or
          (i32.ne (i32.load8_u (local.get $p)) (i32.const 102))
          (i32.ne (i32.load8_u (i32.add (local.get $p) (i32.const 1))) (i32.const 97)))
        (i32.or
          (i32.ne (i32.load8_u (i32.add (local.get $p) (i32.const 2))) (i32.const 108))
          (i32.or
            (i32.ne (i32.load8_u (i32.add (local.get $p) (i32.const 3))) (i32.const 115))
            (i32.ne (i32.load8_u (i32.add (local.get $p) (i32.const 4))) (i32.const 101)))))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )
  (global.set $json_parse_ptr (i32.add (local.get $p) (i32.const 5)))
  (call $prelude.val_from_bool (i32.const 0))
)

(func $json._parse_null (result anyref)
  (local $p i32)
  (if (global.get $json_parse_err)
    (then
      (return (call $prelude.val_undefined))
    )
  )
  (local.set $p (global.get $json_parse_ptr))
  (if (i32.gt_u
        (i32.add (local.get $p) (i32.const 4))
        (global.get $json_parse_end))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )
  (if (i32.or
        (i32.or
          (i32.ne (i32.load8_u (local.get $p)) (i32.const 110))
          (i32.ne (i32.load8_u (i32.add (local.get $p) (i32.const 1))) (i32.const 117)))
        (i32.or
          (i32.ne (i32.load8_u (i32.add (local.get $p) (i32.const 2))) (i32.const 108))
          (i32.ne (i32.load8_u (i32.add (local.get $p) (i32.const 3))) (i32.const 108))))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )
  (global.set $json_parse_ptr (i32.add (local.get $p) (i32.const 4)))
  (call $prelude.val_null)
)

(func $json._parse_number (result anyref)
  (local $start i32)
  (local $p i32)
  (local $c i32)
  (local $has_frac i32)
  (local $has_exp i32)
  (local $len i32)
  (local $i i32)
  (local $neg i32)
  (local $mag i64)
  (local $digit i64)
  (local $overflow i32)
  (local $f f64)

  (if (global.get $json_parse_err)
    (then
      (return (call $prelude.val_undefined))
    )
  )

  (local.set $start (global.get $json_parse_ptr))
  (local.set $p (local.get $start))

  (if (i32.ge_u (local.get $p) (global.get $json_parse_end))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )

  (local.set $c (i32.load8_u (local.get $p)))
  (if (i32.eq (local.get $c) (i32.const 45))
    (then
      (local.set $p (i32.add (local.get $p) (i32.const 1)))
    )
  )
  (if (i32.ge_u (local.get $p) (global.get $json_parse_end))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )

  (local.set $c (i32.load8_u (local.get $p)))
  (if (i32.eq (local.get $c) (i32.const 48))
    (then
      (local.set $p (i32.add (local.get $p) (i32.const 1)))
      (if (i32.and
            (i32.lt_u (local.get $p) (global.get $json_parse_end))
            (call $json._is_digit (i32.load8_u (local.get $p))))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )
    )
    (else
      (if (i32.or
            (i32.lt_u (local.get $c) (i32.const 49))
            (i32.gt_u (local.get $c) (i32.const 57)))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )
      (block $int_done
        (loop $int_loop
          (br_if $int_done
            (i32.ge_u (local.get $p) (global.get $json_parse_end)))
          (br_if $int_done
            (i32.eqz (call $json._is_digit (i32.load8_u (local.get $p)))))
          (local.set $p (i32.add (local.get $p) (i32.const 1)))
          (br $int_loop)
        )
      )
    )
  )

  (local.set $has_frac (i32.const 0))
  (local.set $has_exp (i32.const 0))

  (if (i32.and
        (i32.lt_u (local.get $p) (global.get $json_parse_end))
        (i32.eq (i32.load8_u (local.get $p)) (i32.const 46)))
    (then
      (local.set $has_frac (i32.const 1))
      (local.set $p (i32.add (local.get $p) (i32.const 1)))
      (if (i32.or
            (i32.ge_u (local.get $p) (global.get $json_parse_end))
            (i32.eqz (call $json._is_digit (i32.load8_u (local.get $p)))))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )
      (block $frac_done
        (loop $frac_loop
          (br_if $frac_done
            (i32.ge_u (local.get $p) (global.get $json_parse_end)))
          (br_if $frac_done
            (i32.eqz (call $json._is_digit (i32.load8_u (local.get $p)))))
          (local.set $p (i32.add (local.get $p) (i32.const 1)))
          (br $frac_loop)
        )
      )
    )
  )

  (if (i32.and
        (i32.lt_u (local.get $p) (global.get $json_parse_end))
        (i32.or
          (i32.eq (i32.load8_u (local.get $p)) (i32.const 101))
          (i32.eq (i32.load8_u (local.get $p)) (i32.const 69))))
    (then
      (local.set $has_exp (i32.const 1))
      (local.set $p (i32.add (local.get $p) (i32.const 1)))
      (if (i32.and
            (i32.lt_u (local.get $p) (global.get $json_parse_end))
            (i32.or
              (i32.eq (i32.load8_u (local.get $p)) (i32.const 43))
              (i32.eq (i32.load8_u (local.get $p)) (i32.const 45))))
        (then
          (local.set $p (i32.add (local.get $p) (i32.const 1)))
        )
      )
      (if (i32.or
            (i32.ge_u (local.get $p) (global.get $json_parse_end))
            (i32.eqz (call $json._is_digit (i32.load8_u (local.get $p)))))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )
      (block $exp_done
        (loop $exp_loop
          (br_if $exp_done
            (i32.ge_u (local.get $p) (global.get $json_parse_end)))
          (br_if $exp_done
            (i32.eqz (call $json._is_digit (i32.load8_u (local.get $p)))))
          (local.set $p (i32.add (local.get $p) (i32.const 1)))
          (br $exp_loop)
        )
      )
    )
  )

  (global.set $json_parse_ptr (local.get $p))
  (local.set $len (i32.sub (local.get $p) (local.get $start)))

  (if (i32.or (local.get $has_frac) (local.get $has_exp))
    (then
      (local.set $f (call $json._parse_f64_token (local.get $start) (local.get $len)))
      (return (call $prelude.val_from_f64 (local.get $f)))
    )
  )

  (local.set $i (local.get $start))
  (local.set $neg (i32.const 0))
  (if (i32.eq (i32.load8_u (local.get $i)) (i32.const 45))
    (then
      (local.set $neg (i32.const 1))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
    )
  )
  (local.set $mag (i64.const 0))
  (local.set $overflow (i32.const 0))

  (block $int_parse_done
    (loop $int_parse
      (br_if $int_parse_done (i32.ge_u (local.get $i) (local.get $p)))
      (local.set $digit
        (i64.extend_i32_u
          (i32.sub (i32.load8_u (local.get $i)) (i32.const 48))))

      (if (i64.gt_u (local.get $mag) (i64.const 922337203685477580))
        (then
          (local.set $overflow (i32.const 1))
          (br $int_parse_done)
        )
      )
      (if (i64.eq (local.get $mag) (i64.const 922337203685477580))
        (then
          (if (i32.and
                (local.get $neg)
                (i64.gt_u (local.get $digit) (i64.const 8)))
            (then
              (local.set $overflow (i32.const 1))
              (br $int_parse_done)
            )
          )
          (if (i32.and
                (i32.eqz (local.get $neg))
                (i64.gt_u (local.get $digit) (i64.const 7)))
            (then
              (local.set $overflow (i32.const 1))
              (br $int_parse_done)
            )
          )
        )
      )

      (local.set $mag
        (i64.add
          (i64.mul (local.get $mag) (i64.const 10))
          (local.get $digit)))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $int_parse)
    )
  )

  (if (local.get $overflow)
    (then
      (local.set $f (call $json._parse_f64_token (local.get $start) (local.get $len)))
      (return (call $prelude.val_from_f64 (local.get $f)))
    )
  )

  (if (local.get $neg)
    (then
      (if (i64.eq (local.get $mag) (i64.const -9223372036854775808))
        (then
          (return (call $prelude.val_from_i64 (i64.const -9223372036854775808)))
        )
      )
      (return
        (call $prelude.val_from_i64
          (i64.sub (i64.const 0) (local.get $mag))))
    )
  )

  (call $prelude.val_from_i64 (local.get $mag))
)

(func $json._parse_value (result anyref)
  (local $c i32)
  (if (global.get $json_parse_err)
    (then
      (return (call $prelude.val_undefined))
    )
  )

  (call $json._parse_skip_ws)
  (if (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )

  (local.set $c (i32.load8_u (global.get $json_parse_ptr)))
  (if (i32.eq (local.get $c) (i32.const 34))
    (then
      (return (call $json._parse_string))
    )
  )
  (if (i32.eq (local.get $c) (i32.const 123))
    (then
      (return (call $json._parse_object))
    )
  )
  (if (i32.eq (local.get $c) (i32.const 91))
    (then
      (return (call $json._parse_array))
    )
  )
  (if (i32.eq (local.get $c) (i32.const 116))
    (then
      (return (call $json._parse_true))
    )
  )
  (if (i32.eq (local.get $c) (i32.const 102))
    (then
      (return (call $json._parse_false))
    )
  )
  (if (i32.eq (local.get $c) (i32.const 110))
    (then
      (return (call $json._parse_null))
    )
  )
  (if (i32.or
        (i32.eq (local.get $c) (i32.const 45))
        (call $json._is_digit (local.get $c)))
    (then
      (return (call $json._parse_number))
    )
  )

  (call $json._parse_fail_invalid)
  (call $prelude.val_undefined)
)

(func $json._parse_array (result anyref)
  (local $cap i32)
  (local $tmp anyref)
  (local $out anyref)
  (local $count i32)
  (local $i i32)
  (local $c i32)
  (local $val anyref)

  (if (global.get $json_parse_err)
    (then
      (return (call $prelude.val_undefined))
    )
  )
  (if (i32.or
        (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
        (i32.ne
          (i32.load8_u (global.get $json_parse_ptr))
          (i32.const 91)))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )

  (global.set $json_parse_ptr
    (i32.add (global.get $json_parse_ptr) (i32.const 1)))
  (call $json._parse_skip_ws)

  (if (i32.and
        (i32.lt_u (global.get $json_parse_ptr) (global.get $json_parse_end))
        (i32.eq
          (i32.load8_u (global.get $json_parse_ptr))
          (i32.const 93)))
    (then
      (global.set $json_parse_ptr
        (i32.add (global.get $json_parse_ptr) (i32.const 1)))
      (return (call $prelude.arr_new (i32.const 0)))
    )
  )

  (local.set $cap
    (i32.sub (global.get $json_parse_end) (global.get $json_parse_ptr)))
  (if (i32.lt_s (local.get $cap) (i32.const 1))
    (then
      (local.set $cap (i32.const 1))
    )
  )
  (local.set $tmp (call $prelude.arr_new (local.get $cap)))
  (local.set $count (i32.const 0))

  (block $done
    (loop $loop
      (local.set $val (call $json._parse_value))
      (if (global.get $json_parse_err)
        (then
          (return (call $prelude.val_undefined))
        )
      )
      (call $prelude.arr_set (local.get $tmp) (local.get $count) (local.get $val))
      (local.set $count (i32.add (local.get $count) (i32.const 1)))

      (call $json._parse_skip_ws)
      (if (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )
      (local.set $c (i32.load8_u (global.get $json_parse_ptr)))
      (if (i32.eq (local.get $c) (i32.const 44))
        (then
          (global.set $json_parse_ptr
            (i32.add (global.get $json_parse_ptr) (i32.const 1)))
          (call $json._parse_skip_ws)
          (br $loop)
        )
      )
      (if (i32.eq (local.get $c) (i32.const 93))
        (then
          (global.set $json_parse_ptr
            (i32.add (global.get $json_parse_ptr) (i32.const 1)))
          (br $done)
        )
      )

      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )

  (local.set $out (call $prelude.arr_new (local.get $count)))
  (local.set $i (i32.const 0))
  (block $copy_done
    (loop $copy
      (br_if $copy_done (i32.ge_u (local.get $i) (local.get $count)))
      (call $prelude.arr_set
        (local.get $out)
        (local.get $i)
        (call $prelude.arr_get (local.get $tmp) (local.get $i)))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $copy)
    )
  )

  (local.get $out)
)

(func $json._parse_object (result anyref)
  (local $cap i32)
  (local $keys anyref)
  (local $vals anyref)
  (local $obj anyref)
  (local $count i32)
  (local $i i32)
  (local $j i32)
  (local $c i32)
  (local $key_i anyref)
  (local $key_j anyref)
  (local $tmp anyref)
  (local $val_i anyref)
  (local $val_j anyref)
  (local $key anyref)
  (local $val anyref)

  (if (global.get $json_parse_err)
    (then
      (return (call $prelude.val_undefined))
    )
  )
  (if (i32.or
        (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
        (i32.ne
          (i32.load8_u (global.get $json_parse_ptr))
          (i32.const 123)))
    (then
      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )

  (global.set $json_parse_ptr
    (i32.add (global.get $json_parse_ptr) (i32.const 1)))
  (call $json._parse_skip_ws)

  (if (i32.and
        (i32.lt_u (global.get $json_parse_ptr) (global.get $json_parse_end))
        (i32.eq
          (i32.load8_u (global.get $json_parse_ptr))
          (i32.const 125)))
    (then
      (global.set $json_parse_ptr
        (i32.add (global.get $json_parse_ptr) (i32.const 1)))
      (return (call $prelude.obj_new (i32.const 0)))
    )
  )

  (local.set $cap
    (i32.sub (global.get $json_parse_end) (global.get $json_parse_ptr)))
  (if (i32.lt_s (local.get $cap) (i32.const 1))
    (then
      (local.set $cap (i32.const 1))
    )
  )
  (local.set $keys (call $prelude.arr_new (local.get $cap)))
  (local.set $vals (call $prelude.arr_new (local.get $cap)))
  (local.set $count (i32.const 0))

  (block $done
    (loop $loop
      (if (i32.or
            (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
            (i32.ne
              (i32.load8_u (global.get $json_parse_ptr))
              (i32.const 34)))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )

      (local.set $key (call $json._parse_string))
      (if (global.get $json_parse_err)
        (then
          (return (call $prelude.val_undefined))
        )
      )

      (call $json._parse_skip_ws)
      (if (i32.or
            (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
            (i32.ne
              (i32.load8_u (global.get $json_parse_ptr))
              (i32.const 58)))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )
      (global.set $json_parse_ptr
        (i32.add (global.get $json_parse_ptr) (i32.const 1)))

      (local.set $val (call $json._parse_value))
      (if (global.get $json_parse_err)
        (then
          (return (call $prelude.val_undefined))
        )
      )

      (call $prelude.arr_set (local.get $keys) (local.get $count) (local.get $key))
      (call $prelude.arr_set (local.get $vals) (local.get $count) (local.get $val))
      (local.set $count (i32.add (local.get $count) (i32.const 1)))

      (call $json._parse_skip_ws)
      (if (i32.ge_u (global.get $json_parse_ptr) (global.get $json_parse_end))
        (then
          (call $json._parse_fail_invalid)
          (return (call $prelude.val_undefined))
        )
      )
      (local.set $c (i32.load8_u (global.get $json_parse_ptr)))
      (if (i32.eq (local.get $c) (i32.const 44))
        (then
          (global.set $json_parse_ptr
            (i32.add (global.get $json_parse_ptr) (i32.const 1)))
          (call $json._parse_skip_ws)
          (br $loop)
        )
      )
      (if (i32.eq (local.get $c) (i32.const 125))
        (then
          (global.set $json_parse_ptr
            (i32.add (global.get $json_parse_ptr) (i32.const 1)))
          (br $done)
        )
      )

      (call $json._parse_fail_invalid)
      (return (call $prelude.val_undefined))
    )
  )

  ;; Keep toJSON output deterministic: object keys are sorted.
  (local.set $i (i32.const 0))
  (block $sort_i_done
    (loop $sort_i
      (br_if $sort_i_done (i32.ge_u (local.get $i) (local.get $count)))
      (local.set $j (i32.add (local.get $i) (i32.const 1)))
      (block $sort_j_done
        (loop $sort_j
          (br_if $sort_j_done (i32.ge_u (local.get $j) (local.get $count)))
          (local.set $key_i (call $prelude.arr_get (local.get $keys) (local.get $i)))
          (local.set $key_j (call $prelude.arr_get (local.get $keys) (local.get $j)))
          (if (i32.gt_s (call $json._str_cmp (local.get $key_i) (local.get $key_j)) (i32.const 0))
            (then
              (local.set $tmp (local.get $key_i))
              (call $prelude.arr_set (local.get $keys) (local.get $i) (local.get $key_j))
              (call $prelude.arr_set (local.get $keys) (local.get $j) (local.get $tmp))

              (local.set $val_i (call $prelude.arr_get (local.get $vals) (local.get $i)))
              (local.set $val_j (call $prelude.arr_get (local.get $vals) (local.get $j)))
              (call $prelude.arr_set (local.get $vals) (local.get $i) (local.get $val_j))
              (call $prelude.arr_set (local.get $vals) (local.get $j) (local.get $val_i))
            )
          )
          (local.set $j (i32.add (local.get $j) (i32.const 1)))
          (br $sort_j)
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $sort_i)
    )
  )

  (local.set $obj (call $prelude.obj_new (local.get $count)))
  (local.set $i (i32.const 0))
  (block $build_done
    (loop $build
      (br_if $build_done (i32.ge_u (local.get $i) (local.get $count)))
      (call $prelude.obj_set
        (local.get $obj)
        (call $prelude.arr_get (local.get $keys) (local.get $i))
        (call $prelude.arr_get (local.get $vals) (local.get $i)))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $build)
    )
  )

  (local.get $obj)
)

(func $json.toJSON (param $text anyref) (result anyref)
  (local $kind i32)
  (local $ptr i32)
  (local $len i32)
  (local $value anyref)
  (local $msg anyref)

  (local.set $kind (call $prelude.val_kind (local.get $text)))
  (if (i32.ne (local.get $kind) (i32.const 3))
    (then
      (return (call $json._error_from_msg (call $json._msg_toJSON_expects_string)))
    )
  )

  (local.set $ptr (call $prelude._string_ptr (local.get $text)))
  (local.set $len (call $prelude._string_bytelen (local.get $text)))
  (call $json._parse_reset (local.get $ptr) (local.get $len))
  (call $json._parse_skip_ws)
  (local.set $value (call $json._parse_value))

  (if (i32.eqz (global.get $json_parse_err))
    (then
      (call $json._parse_skip_ws)
      (if (i32.ne (global.get $json_parse_ptr) (global.get $json_parse_end))
        (then
          (call $json._parse_fail_invalid)
        )
      )
    )
  )

  (if (global.get $json_parse_err)
    (then
      (local.set $msg (global.get $json_parse_err_msg))
      (if (ref.is_null (local.get $msg))
        (then
          (local.set $msg (call $json._msg_invalid_json))
        )
      )
      (return (call $json._error_from_msg (local.get $msg)))
    )
  )

  (local.get $value)
)

(func $json.parse (param $text anyref) (param $schema anyref) (result anyref)
  (local $parsed anyref)

  (local.set $parsed (call $json.toJSON (local.get $text)))
  (if (call $json._is_error_object (local.get $parsed))
    (then
      (return (local.get $parsed))
    )
  )

  (call $json.decode (local.get $parsed) (local.get $schema))
)

(func $json._decode_array (param $value anyref) (param $schema anyref) (param $path anyref) (result anyref)
  (local $elem_schema anyref)
  (local $len i32)
  (local $i i32)
  (local $out anyref)
  (local $decoded anyref)
  (local $child anyref)
  (local $child_path anyref)

  (if (i32.ne (call $prelude.val_kind (local.get $value)) (i32.const 5))
    (then
      (call $json._decode_error_at (local.get $path) (call $json._msg_array_expected))
      (return (call $prelude.val_undefined))
    )
  )
  (if (i32.eqz (call $json._obj_has_key (local.get $schema) (call $json._k_elem)))
    (then
      (call $json._decode_invalid_schema_at (local.get $path))
      (return (call $prelude.val_undefined))
    )
  )

  (local.set $elem_schema (call $prelude.obj_get (local.get $schema) (call $json._k_elem)))
  (if (i32.ne (call $prelude.val_kind (local.get $elem_schema)) (i32.const 4))
    (then
      (call $json._decode_invalid_schema_at (local.get $path))
      (return (call $prelude.val_undefined))
    )
  )

  (local.set $len (call $prelude.arr_len (local.get $value)))
  (local.set $out (call $prelude.arr_new (local.get $len)))
  (local.set $i (i32.const 0))
  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $child (call $prelude.arr_get (local.get $value) (local.get $i)))
      (local.set $child_path (call $json._path_index (local.get $path) (local.get $i)))
      (local.set $decoded
        (call $json._decode_with_schema
          (local.get $child)
          (local.get $elem_schema)
          (local.get $child_path)))
      (if (global.get $json_decode_err)
        (then
          (return (call $prelude.val_undefined))
        )
      )
      (call $prelude.arr_set (local.get $out) (local.get $i) (local.get $decoded))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )
  (local.get $out)
)

(func $json._decode_tuple (param $value anyref) (param $schema anyref) (param $path anyref) (result anyref)
  (local $tuple_schema anyref)
  (local $len i32)
  (local $out anyref)
  (local $i i32)
  (local $member_schema anyref)
  (local $child_path anyref)
  (local $decoded anyref)
  (local $child anyref)
  (local $k i32)

  (if (i32.ne (call $prelude.val_kind (local.get $value)) (i32.const 5))
    (then
      (call $json._decode_error_at (local.get $path) (call $json._msg_array_expected))
      (return (call $prelude.val_undefined))
    )
  )

  (if (call $json._obj_has_key (local.get $schema) (call $json._k_tuple))
    (then
      (local.set $tuple_schema
        (call $prelude.obj_get (local.get $schema) (call $json._k_tuple)))
      (local.set $k (call $prelude.val_kind (local.get $tuple_schema)))
      (if (i32.eq (local.get $k) (i32.const 6))
        (then
          (local.set $tuple_schema (call $prelude.arr_new (i32.const 0)))
        )
        (else
          (if (i32.ne (local.get $k) (i32.const 5))
            (then
              (call $json._decode_invalid_schema_at (local.get $path))
              (return (call $prelude.val_undefined))
            )
          )
        )
      )
    )
    (else
      (local.set $tuple_schema (call $prelude.arr_new (i32.const 0)))
    )
  )

  (local.set $len (call $prelude.arr_len (local.get $tuple_schema)))
  (if (i32.ne (call $prelude.arr_len (local.get $value)) (local.get $len))
    (then
      (call $json._decode_error_at (local.get $path) (call $json._msg_tuple_length_mismatch))
      (return (call $prelude.val_undefined))
    )
  )

  (local.set $out (call $prelude.arr_new (local.get $len)))
  (local.set $i (i32.const 0))
  (block $done
    (loop $loop
      (br_if $done (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $member_schema
        (call $prelude.arr_get (local.get $tuple_schema) (local.get $i)))
      (local.set $child (call $prelude.arr_get (local.get $value) (local.get $i)))
      (local.set $child_path (call $json._path_index (local.get $path) (local.get $i)))
      (local.set $decoded
        (call $json._decode_with_schema
          (local.get $child)
          (local.get $member_schema)
          (local.get $child_path)))
      (if (global.get $json_decode_err)
        (then
          (return (call $prelude.val_undefined))
        )
      )
      (call $prelude.arr_set (local.get $out) (local.get $i) (local.get $decoded))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $loop)
    )
  )
  (local.get $out)
)

(func $json._decode_object (param $value anyref) (param $schema anyref) (param $path anyref) (result anyref)
  (local $props anyref)
  (local $index_schema anyref)
  (local $has_index i32)
  (local $value_keys anyref)
  (local $props_len i32)
  (local $value_keys_len i32)
  (local $cap i32)
  (local $out_keys anyref)
  (local $out_vals anyref)
  (local $out_count i32)
  (local $i i32)
  (local $j i32)
  (local $idx i32)
  (local $k i32)
  (local $prop anyref)
  (local $typ anyref)
  (local $name anyref)
  (local $name_path anyref)
  (local $child anyref)
  (local $decoded anyref)
  (local $key anyref)
  (local $obj anyref)
  (local $key_i anyref)
  (local $key_j anyref)
  (local $tmp anyref)
  (local $val_i anyref)
  (local $val_j anyref)

  (if (i32.ne (call $prelude.val_kind (local.get $value)) (i32.const 4))
    (then
      (call $json._decode_error_at (local.get $path) (call $json._msg_object_expected))
      (return (call $prelude.val_undefined))
    )
  )

  (if (call $json._obj_has_key (local.get $schema) (call $json._k_props))
    (then
      (local.set $props (call $prelude.obj_get (local.get $schema) (call $json._k_props)))
      (local.set $k (call $prelude.val_kind (local.get $props)))
      (if (i32.eq (local.get $k) (i32.const 6))
        (then
          (local.set $props (call $prelude.arr_new (i32.const 0)))
        )
        (else
          (if (i32.ne (local.get $k) (i32.const 5))
            (then
              (call $json._decode_invalid_schema_at (local.get $path))
              (return (call $prelude.val_undefined))
            )
          )
        )
      )
    )
    (else
      (local.set $props (call $prelude.arr_new (i32.const 0)))
    )
  )

  (local.set $has_index (i32.const 0))
  (if (call $json._obj_has_key (local.get $schema) (call $json._k_index))
    (then
      (local.set $index_schema
        (call $prelude.obj_get (local.get $schema) (call $json._k_index)))
      (local.set $k (call $prelude.val_kind (local.get $index_schema)))
      (if (i32.eq (local.get $k) (i32.const 6))
        (then
          (local.set $has_index (i32.const 0))
        )
        (else
          (if (i32.ne (local.get $k) (i32.const 4))
            (then
              (call $json._decode_invalid_schema_at (local.get $path))
              (return (call $prelude.val_undefined))
            )
          )
          (local.set $has_index (i32.const 1))
        )
      )
    )
  )

  (local.set $value_keys (call $prelude.obj_keys (local.get $value)))
  (local.set $props_len (call $prelude.arr_len (local.get $props)))
  (local.set $value_keys_len (call $prelude.arr_len (local.get $value_keys)))
  (local.set $cap (i32.add (local.get $props_len) (local.get $value_keys_len)))
  (if (i32.lt_s (local.get $cap) (i32.const 1))
    (then
      (local.set $cap (i32.const 1))
    )
  )
  (local.set $out_keys (call $prelude.arr_new (local.get $cap)))
  (local.set $out_vals (call $prelude.arr_new (local.get $cap)))
  (local.set $out_count (i32.const 0))

  ;; Declared props
  (local.set $i (i32.const 0))
  (block $props_done
    (loop $props_loop
      (br_if $props_done (i32.ge_u (local.get $i) (local.get $props_len)))
      (local.set $prop (call $prelude.arr_get (local.get $props) (local.get $i)))
      (if (i32.ne (call $prelude.val_kind (local.get $prop)) (i32.const 4))
        (then
          (call $json._decode_invalid_schema_at (local.get $path))
          (return (call $prelude.val_undefined))
        )
      )

      (if (i32.eqz (call $json._obj_has_key (local.get $prop) (call $json._str_type)))
        (then
          (call $json._decode_invalid_schema_at (local.get $path))
          (return (call $prelude.val_undefined))
        )
      )
      (local.set $typ (call $prelude.obj_get (local.get $prop) (call $json._str_type)))
      (if (i32.ne (call $prelude.val_kind (local.get $typ)) (i32.const 4))
        (then
          (call $json._decode_invalid_schema_at (local.get $path))
          (return (call $prelude.val_undefined))
        )
      )

      (if (call $json._obj_has_key (local.get $prop) (call $json._k_name))
        (then
          (local.set $name (call $prelude.obj_get (local.get $prop) (call $json._k_name)))
          (if (i32.ne (call $prelude.val_kind (local.get $name)) (i32.const 3))
            (then
              (call $json._decode_invalid_schema_at (local.get $path))
              (return (call $prelude.val_undefined))
            )
          )
        )
        (else
          (local.set $name (call $json._str_empty))
        )
      )

      (local.set $name_path (call $json._path_field (local.get $path) (local.get $name)))

      (if (call $json._obj_has_key (local.get $value) (local.get $name))
        (then
          (local.set $child (call $prelude.obj_get (local.get $value) (local.get $name)))
          (local.set $decoded
            (call $json._decode_with_schema
              (local.get $child)
              (local.get $typ)
              (local.get $name_path)))
          (if (global.get $json_decode_err)
            (then
              (return (call $prelude.val_undefined))
            )
          )
        )
        (else
          (if (call $json._schema_allows_undefined (local.get $typ))
            (then
              (local.set $decoded (call $prelude.val_undefined))
            )
            (else
              (call $json._decode_error_at (local.get $name_path) (call $json._msg_missing_field))
              (return (call $prelude.val_undefined))
            )
          )
        )
      )

      (local.set $idx
        (call $json._arr_find_string
          (local.get $out_keys)
          (local.get $out_count)
          (local.get $name)))
      (if (i32.ge_s (local.get $idx) (i32.const 0))
        (then
          (call $prelude.arr_set (local.get $out_vals) (local.get $idx) (local.get $decoded))
        )
        (else
          (call $prelude.arr_set (local.get $out_keys) (local.get $out_count) (local.get $name))
          (call $prelude.arr_set (local.get $out_vals) (local.get $out_count) (local.get $decoded))
          (local.set $out_count (i32.add (local.get $out_count) (i32.const 1)))
        )
      )

      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $props_loop)
    )
  )

  ;; Index props
  (if (local.get $has_index)
    (then
      (local.set $i (i32.const 0))
      (block $idx_done
        (loop $idx_loop
          (br_if $idx_done (i32.ge_u (local.get $i) (local.get $value_keys_len)))
          (local.set $key (call $prelude.arr_get (local.get $value_keys) (local.get $i)))
          (if (i32.lt_s
                (call $json._arr_find_string
                  (local.get $out_keys)
                  (local.get $out_count)
                  (local.get $key))
                (i32.const 0))
            (then
              (local.set $child (call $prelude.obj_get (local.get $value) (local.get $key)))
              (local.set $name_path (call $json._path_field (local.get $path) (local.get $key)))
              (local.set $decoded
                (call $json._decode_with_schema
                  (local.get $child)
                  (local.get $index_schema)
                  (local.get $name_path)))
              (if (global.get $json_decode_err)
                (then
                  (return (call $prelude.val_undefined))
                )
              )
              (call $prelude.arr_set (local.get $out_keys) (local.get $out_count) (local.get $key))
              (call $prelude.arr_set (local.get $out_vals) (local.get $out_count) (local.get $decoded))
              (local.set $out_count (i32.add (local.get $out_count) (i32.const 1)))
            )
          )
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $idx_loop)
        )
      )
    )
  )

  ;; Sort output keys for deterministic order
  (local.set $i (i32.const 0))
  (block $sort_i_done
    (loop $sort_i
      (br_if $sort_i_done (i32.ge_u (local.get $i) (local.get $out_count)))
      (local.set $j (i32.add (local.get $i) (i32.const 1)))
      (block $sort_j_done
        (loop $sort_j
          (br_if $sort_j_done (i32.ge_u (local.get $j) (local.get $out_count)))
          (local.set $key_i (call $prelude.arr_get (local.get $out_keys) (local.get $i)))
          (local.set $key_j (call $prelude.arr_get (local.get $out_keys) (local.get $j)))
          (if (i32.gt_s (call $json._str_cmp (local.get $key_i) (local.get $key_j)) (i32.const 0))
            (then
              (local.set $tmp (local.get $key_i))
              (call $prelude.arr_set (local.get $out_keys) (local.get $i) (local.get $key_j))
              (call $prelude.arr_set (local.get $out_keys) (local.get $j) (local.get $tmp))

              (local.set $val_i (call $prelude.arr_get (local.get $out_vals) (local.get $i)))
              (local.set $val_j (call $prelude.arr_get (local.get $out_vals) (local.get $j)))
              (call $prelude.arr_set (local.get $out_vals) (local.get $i) (local.get $val_j))
              (call $prelude.arr_set (local.get $out_vals) (local.get $j) (local.get $val_i))
            )
          )
          (local.set $j (i32.add (local.get $j) (i32.const 1)))
          (br $sort_j)
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $sort_i)
    )
  )

  (local.set $obj (call $prelude.obj_new (local.get $out_count)))
  (local.set $i (i32.const 0))
  (block $build_done
    (loop $build
      (br_if $build_done (i32.ge_u (local.get $i) (local.get $out_count)))
      (call $prelude.obj_set
        (local.get $obj)
        (call $prelude.arr_get (local.get $out_keys) (local.get $i))
        (call $prelude.arr_get (local.get $out_vals) (local.get $i)))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $build)
    )
  )
  (local.get $obj)
)

(func $json._decode_with_schema (param $value anyref) (param $schema anyref) (param $path anyref) (result anyref)
  (local $kind i32)
  (local $kind_str anyref)
  (local $literal anyref)
  (local $has_literal i32)
  (local $b i32)
  (local $f f64)
  (local $out anyref)
  (local $want_str anyref)
  (local $want_f f64)
  (local $want_i i64)
  (local $value_i i64)
  (local $union anyref)
  (local $union_len i32)
  (local $i i32)
  (local $member anyref)
  (local $decoded anyref)
  (local $last_msg anyref)
  (local $k i32)

  (if (global.get $json_decode_err)
    (then
      (return (call $prelude.val_undefined))
    )
  )

  (if (i32.ne (call $prelude.val_kind (local.get $schema)) (i32.const 4))
    (then
      (call $json._decode_invalid_schema_at (local.get $path))
      (return (call $prelude.val_undefined))
    )
  )

  (if (call $json._obj_has_key (local.get $schema) (call $json._k_kind))
    (then
      (local.set $kind_str (call $prelude.obj_get (local.get $schema) (call $json._k_kind)))
      (if (i32.ne (call $prelude.val_kind (local.get $kind_str)) (i32.const 3))
        (then
          (call $json._decode_invalid_schema_at (local.get $path))
          (return (call $prelude.val_undefined))
        )
      )
      (local.set $kind (call $json._schema_kind_code (local.get $kind_str)))
    )
    (else
      (local.set $kind (i32.const 0))
    )
  )

  ;; literal
  (local.set $has_literal (i32.const 0))
  (if (call $json._obj_has_key (local.get $schema) (call $json._k_literal))
    (then
      (local.set $literal
        (call $prelude.obj_get (local.get $schema) (call $json._k_literal)))
      (local.set $k (call $prelude.val_kind (local.get $literal)))
      (if (i32.eq (local.get $k) (i32.const 6))
        (then
          (local.set $has_literal (i32.const 0))
        )
        (else
          (if (i32.ne (local.get $k) (i32.const 4))
            (then
              (call $json._decode_invalid_schema_at (local.get $path))
              (return (call $prelude.val_undefined))
            )
          )
          (local.set $has_literal (i32.const 1))
        )
      )
    )
  )

  ;; json
  (if (i32.eq (local.get $kind) (i32.const 1))
    (then
      (return (local.get $value))
    )
  )

  ;; undefined
  (if (i32.eq (local.get $kind) (i32.const 2))
    (then
      (if (i32.ne (call $prelude.val_kind (local.get $value)) (i32.const 7))
        (then
          (call $json._decode_error_at (local.get $path) (call $json._msg_undefined_expected))
          (return (call $prelude.val_undefined))
        )
      )
      (return (local.get $value))
    )
  )

  ;; null
  (if (i32.eq (local.get $kind) (i32.const 3))
    (then
      (if (i32.ne (call $prelude.val_kind (local.get $value)) (i32.const 6))
        (then
          (call $json._decode_error_at (local.get $path) (call $json._msg_null_expected))
          (return (call $prelude.val_undefined))
        )
      )
      (return (local.get $value))
    )
  )

  ;; string
  (if (i32.eq (local.get $kind) (i32.const 4))
    (then
      (if (i32.ne (call $prelude.val_kind (local.get $value)) (i32.const 3))
        (then
          (call $json._decode_error_at (local.get $path) (call $json._msg_string_expected))
          (return (call $prelude.val_undefined))
        )
      )
      (if (local.get $has_literal)
        (then
          (local.set $want_str (call $json._literal_as_string (local.get $literal)))
          (if (i32.eqz (call $prelude.str_eq (local.get $value) (local.get $want_str)))
            (then
              (call $json._decode_error_at (local.get $path) (call $json._msg_string_literal_mismatch))
              (return (call $prelude.val_undefined))
            )
          )
        )
      )
      (return (local.get $value))
    )
  )

  ;; boolean
  (if (i32.eq (local.get $kind) (i32.const 5))
    (then
      (if (i32.ne (call $prelude.val_kind (local.get $value)) (i32.const 2))
        (then
          (call $json._decode_error_at (local.get $path) (call $json._msg_boolean_expected))
          (return (call $prelude.val_undefined))
        )
      )
      (if (local.get $has_literal)
        (then
          (local.set $b (call $json._literal_as_bool (local.get $literal)))
          (if (i32.ne (call $prelude.val_to_bool (local.get $value)) (local.get $b))
            (then
              (call $json._decode_error_at (local.get $path) (call $json._msg_boolean_literal_mismatch))
              (return (call $prelude.val_undefined))
            )
          )
        )
      )
      (return (local.get $value))
    )
  )

  ;; integer
  (if (i32.eq (local.get $kind) (i32.const 6))
    (then
      (local.set $k (call $prelude.val_kind (local.get $value)))
      (if (i32.eq (local.get $k) (i32.const 0))
        (then
          (local.set $out (local.get $value))
        )
        (else
          (if (i32.eq (local.get $k) (i32.const 1))
            (then
              (local.set $f (call $prelude.val_to_f64 (local.get $value)))
              (if (i32.or
                    (f64.ne (local.get $f) (local.get $f))
                    (i32.or
                      (f64.eq (local.get $f) (f64.const inf))
                      (f64.eq (local.get $f) (f64.const -inf))))
                (then
                  (call $json._decode_error_at (local.get $path) (call $json._msg_invalid_number))
                  (return (call $prelude.val_undefined))
                )
              )
              (if (f64.ne (f64.trunc (local.get $f)) (local.get $f))
                (then
                  (call $json._decode_error_at (local.get $path) (call $json._msg_integer_expected))
                  (return (call $prelude.val_undefined))
                )
              )
              (if (i32.or
                    (f64.lt (local.get $f) (f64.const -9.223372036854776e18))
                    (f64.gt (local.get $f) (f64.const 9.223372036854776e18)))
                (then
                  (call $json._decode_error_at (local.get $path) (call $json._msg_integer_out_of_range))
                  (return (call $prelude.val_undefined))
                )
              )
              (local.set $out
                (call $prelude.val_from_i64 (i64.trunc_sat_f64_s (local.get $f))))
            )
            (else
              (call $json._decode_error_at (local.get $path) (call $json._msg_integer_expected))
              (return (call $prelude.val_undefined))
            )
          )
        )
      )
      (if (local.get $has_literal)
        (then
          (local.set $want_f (call $json._literal_as_f64 (local.get $literal)))
          (local.set $want_i (i64.trunc_sat_f64_s (local.get $want_f)))
          (local.set $value_i (call $prelude.val_to_i64 (local.get $out)))
          (if (i64.ne (local.get $value_i) (local.get $want_i))
            (then
              (call $json._decode_error_at (local.get $path) (call $json._msg_integer_literal_mismatch))
              (return (call $prelude.val_undefined))
            )
          )
        )
      )
      (return (local.get $out))
    )
  )

  ;; number
  (if (i32.eq (local.get $kind) (i32.const 7))
    (then
      (local.set $k (call $prelude.val_kind (local.get $value)))
      (if (i32.eq (local.get $k) (i32.const 1))
        (then
          (local.set $f (call $prelude.val_to_f64 (local.get $value)))
          (if (i32.or
                (f64.ne (local.get $f) (local.get $f))
                (i32.or
                  (f64.eq (local.get $f) (f64.const inf))
                  (f64.eq (local.get $f) (f64.const -inf))))
            (then
              (call $json._decode_error_at (local.get $path) (call $json._msg_invalid_number))
              (return (call $prelude.val_undefined))
            )
          )
          (local.set $out (local.get $value))
        )
        (else
          (if (i32.eq (local.get $k) (i32.const 0))
            (then
              (local.set $out
                (call $prelude.val_from_f64
                  (f64.convert_i64_s (call $prelude.val_to_i64 (local.get $value)))))
            )
            (else
              (call $json._decode_error_at (local.get $path) (call $json._msg_number_expected))
              (return (call $prelude.val_undefined))
            )
          )
        )
      )
      (if (local.get $has_literal)
        (then
          (local.set $want_f (call $json._literal_as_f64 (local.get $literal)))
          (local.set $f (call $prelude.val_to_f64 (local.get $out)))
          (if (f64.ne (local.get $f) (local.get $want_f))
            (then
              (call $json._decode_error_at (local.get $path) (call $json._msg_number_literal_mismatch))
              (return (call $prelude.val_undefined))
            )
          )
        )
      )
      (return (local.get $out))
    )
  )

  ;; array
  (if (i32.eq (local.get $kind) (i32.const 8))
    (then
      (return (call $json._decode_array (local.get $value) (local.get $schema) (local.get $path)))
    )
  )

  ;; tuple
  (if (i32.eq (local.get $kind) (i32.const 9))
    (then
      (return (call $json._decode_tuple (local.get $value) (local.get $schema) (local.get $path)))
    )
  )

  ;; object
  (if (i32.eq (local.get $kind) (i32.const 10))
    (then
      (return (call $json._decode_object (local.get $value) (local.get $schema) (local.get $path)))
    )
  )

  ;; union
  (if (i32.eq (local.get $kind) (i32.const 11))
    (then
      (if (call $json._obj_has_key (local.get $schema) (call $json._k_union))
        (then
          (local.set $union
            (call $prelude.obj_get (local.get $schema) (call $json._k_union)))
          (local.set $k (call $prelude.val_kind (local.get $union)))
          (if (i32.eq (local.get $k) (i32.const 6))
            (then
              (local.set $union (call $prelude.arr_new (i32.const 0)))
            )
            (else
              (if (i32.ne (local.get $k) (i32.const 5))
                (then
                  (call $json._decode_invalid_schema_at (local.get $path))
                  (return (call $prelude.val_undefined))
                )
              )
            )
          )
        )
        (else
          (local.set $union (call $prelude.arr_new (i32.const 0)))
        )
      )

      (local.set $last_msg (ref.null any))
      (local.set $union_len (call $prelude.arr_len (local.get $union)))
      (local.set $i (i32.const 0))
      (block $union_done
        (loop $union_loop
          (br_if $union_done (i32.ge_u (local.get $i) (local.get $union_len)))
          (local.set $member (call $prelude.arr_get (local.get $union) (local.get $i)))
          (global.set $json_decode_err (i32.const 0))
          (global.set $json_decode_err_msg (ref.null any))
          (local.set $decoded
            (call $json._decode_with_schema
              (local.get $value)
              (local.get $member)
              (local.get $path)))
          (if (i32.eqz (global.get $json_decode_err))
            (then
              (return (local.get $decoded))
            )
          )
          (local.set $last_msg (global.get $json_decode_err_msg))
          (local.set $i (i32.add (local.get $i) (i32.const 1)))
          (br $union_loop)
        )
      )

      (if (ref.is_null (local.get $last_msg))
        (then
          (call $json._decode_error_at (local.get $path) (call $json._msg_union_expected))
        )
        (else
          (call $json._decode_force_error (local.get $last_msg))
        )
      )
      (return (call $prelude.val_undefined))
    )
  )

  (if (i32.eq (local.get $kind) (i32.const 0))
    (then
      (call $json._decode_error_at (local.get $path) (call $json._msg_unsupported_schema_kind))
      (return (call $prelude.val_undefined))
    )
  )

  (call $json._decode_error_at (local.get $path) (call $json._msg_unsupported_schema_kind))
  (call $prelude.val_undefined)
)

(func $json.decode (param $json anyref) (param $schema anyref) (result anyref)
  (local $schema_parsed anyref)
  (local $decoded anyref)

  (if (i32.ne (call $prelude.val_kind (local.get $schema)) (i32.const 3))
    (then
      (return
        (call $json._error_from_msg
          (call $json._msg_decode_expects_schema_string)))
    )
  )

  (local.set $schema_parsed (call $json.toJSON (local.get $schema)))
  (if (call $json._is_error_object (local.get $schema_parsed))
    (then
      (return (call $json._error_from_msg (call $json._msg_invalid_schema)))
    )
  )
  (if (i32.ne (call $prelude.val_kind (local.get $schema_parsed)) (i32.const 4))
    (then
      (return (call $json._error_from_msg (call $json._msg_invalid_schema)))
    )
  )

  (call $json._decode_reset)
  (local.set $decoded
    (call $json._decode_with_schema
      (local.get $json)
      (local.get $schema_parsed)
      (call $json._str_dollar)))

  (if (global.get $json_decode_err)
    (then
      (return (call $json._error_from_msg (global.get $json_decode_err_msg)))
    )
  )

  (local.get $decoded)
)
