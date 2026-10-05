---
stage: evergreen
created: 2026-02-12
updated: 2026-10-01
tags: [kitchen/bread]
image: "[[kitchen-cover.png]]"
---
![[loaf.svg|right|140]] A plain country loaf. Flour, water, salt and a starter; the rest is time.

## Ingredients

| | Weight | Baker's % |
|---|---|---|
| bread flour | 450 g | 90 % |
| wholemeal flour | 50 g | 10 % |
| water | 350 g | 70 % |
| active starter | 100 g | 20 % |
| salt | 10 g | 2 % |

Baker's percentages are relative to the total flour, so hydration is

$$
h = \frac{\text{water}}{\text{flour}} = \frac{350}{500} = 70\,\%
$$

## Schedule

| Time | Step |
|---|---|
| 09:00 | feed the starter |
| 13:00 | mix flour and water, rest (autolyse) |
| 14:00 | add starter and salt |
| 14:30–16:30 | four sets of stretch and folds |
| 16:30–20:00 | bulk ferment until it grows by half |
| 20:00 | shape, into a basket, fridge overnight |
| 08:00 | bake 20 min lid on, 25 min lid off, at 240 °C |

## Keeping the starter

> [!tip] The float test
> Drop a spoonful of starter in water. If it floats, it's ready to bake with.

> [!failure]- When it goes wrong
> - **Flat loaf:** the bulk ferment was too short, or the starter too weak.
> - **Huge holes under the crust:** under-proofed.
> - **Sour and dense:** over-proofed, or the kitchen was too warm.

## Scaling the recipe

```python
def scale(recipe: dict[str, float], flour: float) -> dict[str, float]:
    """Scale a recipe given in baker's percentages to a flour weight."""
    return {name: round(flour * pct / 100) for name, pct in recipe.items()}

print(scale({"water": 70, "starter": 20, "salt": 2}, flour=1000))
# {'water': 700, 'starter': 200, 'salt': 20}
```
