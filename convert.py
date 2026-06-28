#!/usr/bin/env python3
"""Convert video frames to ASCII and stream to stdout.

Protocol:
  1. First line: JSON metadata {"fps", "width", "height", "frames"}
  2. Then raw binary: each frame is exactly width*height bytes of ASCII chars
"""
# ponytail: single script, binary stdout, no frills
import sys
import json
import argparse

import cv2
import numpy as np

ASCII_CHARS = " .,:;+*?%S#@"


def main():
    p = argparse.ArgumentParser(description="Video to ASCII converter")
    p.add_argument("--video", required=True, help="Path to video file")
    p.add_argument("--width", type=int, default=120, help="ASCII output width in characters")
    args = p.parse_args()

    cap = cv2.VideoCapture(args.video)
    if not cap.isOpened():
        sys.stderr.write(f"error: cannot open video: {args.video}\n")
        sys.exit(1)

    fps = cap.get(cv2.CAP_PROP_FPS) or 24.0
    total = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
    orig_w = cap.get(cv2.CAP_PROP_FRAME_WIDTH)
    orig_h = cap.get(cv2.CAP_PROP_FRAME_HEIGHT)
    if orig_w == 0 or orig_h == 0:
        sys.stderr.write("error: cannot read video dimensions\n")
        sys.exit(1)

    h = max(1, int(args.width * 0.55 * orig_h / orig_w))

    meta = json.dumps({"fps": fps, "width": args.width, "height": h, "frames": total})
    sys.stdout.buffer.write((meta + "\n").encode())
    sys.stdout.buffer.flush()

    scale = len(ASCII_CHARS) - 1
    # Create lookup array for fast indexing
    ascii_arr = np.array([ord(c) for c in ASCII_CHARS], dtype=np.uint8)

    while True:
        ret, frame = cap.read()
        if not ret:
            break
        frame = cv2.resize(frame, (args.width, h))
        gray = cv2.cvtColor(frame, cv2.COLOR_BGR2GRAY)
        
        # Vectorized pixel-to-ASCII conversion
        indices = (gray.astype(np.uint16) * scale // 255).astype(np.uint8)
        ascii_frame = ascii_arr[indices]
        
        sys.stdout.buffer.write(ascii_frame.tobytes())
        sys.stdout.buffer.flush()

    cap.release()


if __name__ == "__main__":
    main()
