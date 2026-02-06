;; Prelude functions implemented as raw WAT module fragments.
;; This file is injected into the generated module at compile time.

(func $prelude.stringLength (param $str externref) (result i64)
  (call $prelude.str_len (local.get $str))
)
