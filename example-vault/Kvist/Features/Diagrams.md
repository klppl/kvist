---
stage: budding
created: 2026-09-06
tags: [kvist/features]
---
` ```mermaid ` blocks are drawn as diagrams. Mermaid only loads on pages that have one, and redraws when you switch between light and dark.

## Flowchart

```mermaid
flowchart LR
  Seed --> Seedling --> Plant --> Flower --> Fruit --> Seed
```

## Sequence

```mermaid
sequenceDiagram
  Obsidian->>kvist: push changed notes
  kvist->>kvist: check rules, build site
  kvist-->>Obsidian: published
```

## Timeline

```mermaid
timeline
  title The tomato year
  March : sow indoors
  May : plant out
  July : first fruit
  September : save seed
```

## Gantt

```mermaid
gantt
  dateFormat YYYY-MM-DD
  title Garden jobs
  section Beds
  Rebuild bed 3   :done, 2026-03-01, 14d
  Plant out       :done, 2026-05-15, 7d
  section Harvest
  Tomatoes        :active, 2026-07-15, 75d
  Garlic          :2026-07-01, 10d
```

More: [[How publishing works]], [[Composting]], [[Linking over filing]].
