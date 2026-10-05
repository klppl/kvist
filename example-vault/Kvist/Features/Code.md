---
stage: evergreen
created: 2026-09-04
tags: [kvist/features, code]
---
Fenced code blocks are highlighted on the server, in light and dark styles. `inline code` is just monospace.

```go
// Go
func fib(n int) int {
	if n < 2 {
		return n
	}
	return fib(n-1) + fib(n-2)
}
```

```typescript
// TypeScript
interface Plant { name: string; daysToHarvest: number }
const ready = (p: Plant, sown: Date): Date =>
  new Date(sown.getTime() + p.daysToHarvest * 86_400_000);
```

```rust
// Rust
fn main() {
    let beds = ["tomatoes", "squash", "garlic", "leeks"];
    for (i, bed) in beds.iter().enumerate() {
        println!("bed {}: {bed}", i + 1);
    }
}
```

```sql
-- SQL
SELECT variety, AVG(weight_g) AS avg_weight
FROM harvest
WHERE crop = 'tomato'
GROUP BY variety
ORDER BY avg_weight DESC;
```

```bash
# Shell
kvist dev --dir example-vault --config kvist.example.toml
```

```json
{"crop": "tomato", "variety": "Sungold", "weight_g": 14}
```

```css
.content mark { background: var(--mark); }
```

A block without a language is shown as plain text:

```
just text, no colors
```

More real code is in [[Weather station]] and [[Sourdough#Scaling the recipe]].
