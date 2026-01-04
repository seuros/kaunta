# dictator-datastar

WASM decree for [Dictator](https://github.com/seuros/dictator) that lints [Datastar](https://data-star.dev) HTML attributes.

## Rules

| Rule | Description |
|------|-------------|
| `datastar/no-alpine-vue-attrs` | Disallows Alpine.js/Vue.js style attributes (`x-*`, `v-*`, `@*`, `:*`) |
| `datastar/require-value` | Requires values for expression-based attributes |
| `datastar/for-template` | Requires `data-for` on `<template>` elements |
| `datastar/typo` | Detects common typos (`data-intersects` → `data-on-intersect`) |
| `datastar/invalid-modifier` | Validates modifier syntax (`__debounce.500ms`, `__once`) |
| `datastar/action-syntax` | Validates `@get()`, `@post()` SSE action syntax |

## Examples

### Valid Datastar

```html
<div data-signals:count="0"
     data-show="$count > 0"
     data-on:click="$count++">
  Count: <span data-text="$count"></span>
</div>

<button data-on:click__debounce.300ms="@post('/submit')">
  Submit
</button>

<template data-for="item in $items">
  <li data-text="$item.name"></li>
</template>
```

### Violations

```html
<!-- datastar/no-alpine-vue-attrs -->
<div x-show="visible">        <!-- Use data-show -->
<div @click="handle()">       <!-- Use data-on:click -->

<!-- datastar/typo -->
<div data-intersects="...">   <!-- Use data-on-intersect -->
<div data-on-click="...">     <!-- Use data-on:click (colon, not hyphen) -->

<!-- datastar/for-template -->
<div data-for="item in $items">  <!-- Must be on <template> -->

<!-- datastar/action-syntax -->
<button data-on:click="@get">    <!-- Missing parentheses: @get('/path') -->
<button data-on:click="@get()">  <!-- Empty URL not allowed -->

<!-- datastar/invalid-modifier -->
<div data-on:click__unknown="...">  <!-- Unknown modifier -->
```

## Attribute Order

This decree does **not** enforce attribute ordering. Datastar processes attributes in DOM order, and the order is semantic (dependency-based), not stylistic. For example:

```html
<!-- Signal must be defined before it's used -->
<div data-signals:foo="1" data-show="$foo">  <!-- Correct -->
<div data-show="$foo" data-signals:foo="1">  <!-- Also valid if foo exists elsewhere -->
```

The correct order depends on your specific use case and signal dependencies.

## Building

Requires Rust with `wasm32-wasip2` target:

```bash
rustup target add wasm32-wasip2
cargo build --release --target wasm32-wasip2
cp target/wasm32-wasip2/release/dictator_datastar.wasm dist/dictator-datastar.component.wasm
```

## Testing

```bash
cargo test
```

## Configuration

All rules are enabled by default. The decree uses `DatastarConfig` internally:

```rust
DatastarConfig {
    check_alpine_vue: true,
    check_required_values: true,
    check_typos: true,
    check_modifiers: true,
    check_actions: true,
    check_for_template: true,
}
```

## Supported Modifiers

### Event modifiers (`data-on:*`)
`__once`, `__passive`, `__capture`, `__debounce`, `__throttle`, `__delay`, `__window`, `__outside`, `__prevent`, `__stop`, `__viewtransition`

### Intersect modifiers (`data-on-intersect`)
`__once`, `__half`, `__full`, `__threshold`

### Persist modifiers (`data-persist`)
`__session`

### Init modifiers (`data-init`)
`__delay`, `__viewtransition`

### Case modifiers (many attributes)
`__case.camel`, `__case.kebab`, `__case.snake`, `__case.pascal`

## License

MIT
