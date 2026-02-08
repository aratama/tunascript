;; Array module functions implemented in WAT for GC backend.

(func $array.range (param $start i64) (param $end i64) (result anyref)
  (local $delta i64)
  (local $len i32)
  (local $i i32)
  (local $out anyref)

  (if (i64.lt_s (local.get $end) (local.get $start))
    (then
      (return (call $prelude.arr_new (i32.const 0)))
    )
  )

  (local.set $delta (i64.sub (local.get $end) (local.get $start)))

  (if (i64.lt_s (local.get $delta) (i64.const 0))
    (then
      (return (call $prelude.arr_new (i32.const 0)))
    )
  )
  (if (i64.ge_s (local.get $delta) (i64.const 2147483647))
    (then
      (return (call $prelude.arr_new (i32.const 0)))
    )
  )

  (local.set $len
    (i32.wrap_i64 (i64.add (local.get $delta) (i64.const 1))))
  (local.set $out (call $prelude.arr_new (local.get $len)))
  (local.set $i (i32.const 0))

  (block $range_end
    (loop $range_loop
      (br_if $range_end (i32.ge_u (local.get $i) (local.get $len)))
      (call $prelude.arr_set
        (local.get $out)
        (local.get $i)
        (call $prelude.val_from_i64
          (i64.add
            (local.get $start)
            (i64.extend_i32_u (local.get $i)))))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $range_loop)
    )
  )

  (local.get $out)
)

(func $array.length (param $arr anyref) (result i64)
  (i64.extend_i32_u (call $prelude.arr_len (local.get $arr)))
)

(func $array.map (param $arr anyref) (param $fn anyref) (result anyref)
  (local $len i32)
  (local $i i32)
  (local $out anyref)
  (local $value anyref)
  (local $args anyref)

  (local.set $len (call $prelude.arr_len (local.get $arr)))
  (local.set $out (call $prelude.arr_new (local.get $len)))
  (local.set $i (i32.const 0))

  (block $map_end
    (loop $map_loop
      (br_if $map_end (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $value
        (call $prelude.arr_get (local.get $arr) (local.get $i)))
      (local.set $args (call $prelude.arr_new (i32.const 1)))
      (call $prelude.arr_set (local.get $args) (i32.const 0) (local.get $value))
      (local.set $value (call $prelude.call_fn (local.get $fn) (local.get $args)))
      (call $prelude.arr_set (local.get $out) (local.get $i) (local.get $value))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $map_loop)
    )
  )

  (local.get $out)
)

(func $array.filter (param $arr anyref) (param $fn anyref) (result anyref)
  (local $len i32)
  (local $i i32)
  (local $count i32)
  (local $out anyref)
  (local $out_i i32)
  (local $value anyref)
  (local $args anyref)
  (local $keep i32)

  (local.set $len (call $prelude.arr_len (local.get $arr)))
  (local.set $i (i32.const 0))
  (local.set $count (i32.const 0))

  (block $filter_count_end
    (loop $filter_count_loop
      (br_if $filter_count_end (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $value
        (call $prelude.arr_get (local.get $arr) (local.get $i)))
      (local.set $args (call $prelude.arr_new (i32.const 1)))
      (call $prelude.arr_set (local.get $args) (i32.const 0) (local.get $value))
      (local.set $keep
        (call $prelude.val_to_bool
          (call $prelude.call_fn (local.get $fn) (local.get $args))))
      (if (i32.ne (local.get $keep) (i32.const 0))
        (then
          (local.set $count (i32.add (local.get $count) (i32.const 1)))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $filter_count_loop)
    )
  )

  (local.set $out (call $prelude.arr_new (local.get $count)))
  (local.set $i (i32.const 0))
  (local.set $out_i (i32.const 0))

  (block $filter_end
    (loop $filter_loop
      (br_if $filter_end (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $value
        (call $prelude.arr_get (local.get $arr) (local.get $i)))
      (local.set $args (call $prelude.arr_new (i32.const 1)))
      (call $prelude.arr_set (local.get $args) (i32.const 0) (local.get $value))
      (local.set $keep
        (call $prelude.val_to_bool
          (call $prelude.call_fn (local.get $fn) (local.get $args))))
      (if (i32.ne (local.get $keep) (i32.const 0))
        (then
          (call $prelude.arr_set
            (local.get $out)
            (local.get $out_i)
            (local.get $value))
          (local.set $out_i (i32.add (local.get $out_i) (i32.const 1)))
        )
      )
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $filter_loop)
    )
  )

  (local.get $out)
)

(func $array.reduce (param $arr anyref) (param $fn anyref) (param $initial anyref) (result anyref)
  (local $len i32)
  (local $i i32)
  (local $acc anyref)
  (local $value anyref)
  (local $args anyref)

  (local.set $len (call $prelude.arr_len (local.get $arr)))
  (local.set $acc (local.get $initial))
  (local.set $i (i32.const 0))

  (block $reduce_end
    (loop $reduce_loop
      (br_if $reduce_end (i32.ge_u (local.get $i) (local.get $len)))
      (local.set $value
        (call $prelude.arr_get (local.get $arr) (local.get $i)))
      (local.set $args (call $prelude.arr_new (i32.const 2)))
      (call $prelude.arr_set (local.get $args) (i32.const 0) (local.get $acc))
      (call $prelude.arr_set (local.get $args) (i32.const 1) (local.get $value))
      (local.set $acc (call $prelude.call_fn (local.get $fn) (local.get $args)))
      (local.set $i (i32.add (local.get $i) (i32.const 1)))
      (br $reduce_loop)
    )
  )

  (local.get $acc)
)
