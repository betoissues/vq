---
base: &b hello
ref: *b
defaults: &d
  x: 1
obj:
  <<: *d
  y: 2
---
Body.
