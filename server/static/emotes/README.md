# Emotes

Custom image emotes for Grunt. Supported formats: `.svg`, `.png`, `.gif`, `.webp`.

## Converting GIF to WebP

```bash
ffmpeg -i input.gif -vf "scale=32:32,format=yuv420p,colorkey=white:similarity=0.05:blend=0" \
  -c:v libwebp -loop 0 output.webp
```

Parameters:
- `scale=32:32` — resize to emote dimensions (use 64x64 for larger emotes)
- `colorkey=white:similarity=0.05:blend=0` — remove white backgrounds
- `-loop 0` — infinite loop in animated WebP

For non-square GIFs, add cropping/padding before scaling:
```bash
ffmpeg -i input.gif -vf "scale=220:165,crop=min(iw\,ih):min(iw\,ih):(iw-ih)/2:0,scale=32:32" \
  -c:v libwebp -loop 0 output.webp
```

## Naming

Filenames become emote names. `catjam.webp` → `:catjam:`. Only alphanumeric characters and hyphens are allowed in filenames.