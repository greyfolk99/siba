<!-- @doc edge-test -->
<!-- @const empty-str = "" -->
<!-- @const zero = 0 -->
<!-- @const flag = false -->
<!-- @const special = "hello world" -->
# Edge Cases

## Empty/Falsy Values
empty: [{{empty-str}}]
zero: {{zero}}
false: {{flag}}

## Escape
literal: \{{not-a-var}}
mixed: {{zero}} and \{{also-literal}}

## Special Characters
special: {{special}}
