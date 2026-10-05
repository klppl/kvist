---
stage: budding
created: 2026-10-04
tags: [kvist/features]
---
Write a YouTube or Vimeo link as an image and it becomes a player:

```markdown
![Big Buck Bunny](https://www.youtube.com/watch?v=aqz-KE-bpKQ)
```

![Big Buck Bunny](https://www.youtube.com/watch?v=aqz-KE-bpKQ)

YouTube videos play from `youtube-nocookie.com`, which sets no tracking cookies until you press play, and the player only loads when it scrolls into view. A start time in the link (`?t=1m30s`) is kept.

Vimeo works the same way, with *do not track* turned on:

![A Vimeo video](https://vimeo.com/76979871)

A plain link stays a link: [watch on YouTube](https://www.youtube.com/watch?v=aqz-KE-bpKQ).

Video files in the vault play in the browser's own player: `![[clip.mp4]]`.
