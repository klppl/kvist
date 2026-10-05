---
stage: budding
created: 2026-09-10
tags: [kvist/features]
---
The **Graph** in the menu draws every published note as a dot and every link as a line. Drag to move, scroll to zoom, click a dot to open the note. The small *Connections* graph beside each note shows its neighbourhood.

Theme settings in the settings note change it:

| Setting | Effect |
|---|---|
| `graph_tags: true` | tags become nodes, linked to their notes |
| `graph_orphans: false` | hide notes without links |
| `graph_node_size: same` | all dots the same size |
| `graph_link_distance: 80` | spread the graph out |

Private notes never appear in the graph, not even as empty dots.
