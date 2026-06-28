<div align="center">
  <h1>🎬 ASCII Zen</h1>
  <p><strong>A high-performance, terminal-based video player that renders real-time ASCII art.</strong></p>

  <p>
    <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat-square&logo=go" alt="Go Version" /></a>
    <a href="https://python.org/"><img src="https://img.shields.io/badge/Python-3.8+-3776AB?style=flat-square&logo=python" alt="Python Version" /></a>
    <a href="https://github.com/githubuser2777/ASCII_Zen/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue?style=flat-square" alt="License" /></a>
  </p>
</div>

---

## 📖 Overview

**ASCII Zen** is an interactive, hybrid application that bridges the gap between modern video playback and retro terminal aesthetics. By leveraging the robust video decoding capabilities of Python's OpenCV and the blazing-fast terminal UI rendering of Go, ASCII Zen delivers a seamless, cross-platform ASCII video experience.

Whether you want to watch a movie in your terminal or compile an animation into a standalone, zero-dependency executable, ASCII Zen provides the tools to do so elegantly.

## ✨ Key Features

- **Blazing Fast Conversion**: Utilizes vectorized `NumPy` operations to map video frames to ASCII characters at lightning speed.
- **Interactive Terminal UI**: Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea), offering a highly responsive, modern terminal interface.
- **Standalone Export**: Compile any converted video into a standalone `.exe` that runs immediately on any Windows machine—no Python, Go, or external dependencies required.
- **VLC-Style Playback Controls**: Full support for play, pause, and seeking (forward/backward) directly within your terminal.
- **Cross-Platform Foundation**: Core logic is designed to be adaptable across major operating systems.

---

## 🚀 Getting Started

### System Requirements

To build and run ASCII Zen in *Manager Mode* (where you convert new videos), you need the following installed:

*   **[Go](https://go.dev/dl/)** (v1.21 or newer)
*   **[Python](https://www.python.org/downloads/)** (v3.8 or newer)

### Installation

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/githubuser2777/ASCII_Zen.git
    cd ASCII_Zen
    ```

2.  **Install Python dependencies:**
    ASCII Zen relies on OpenCV for video decoding and NumPy for high-speed matrix calculations.
    ```bash
    pip install opencv-python numpy
    ```

3.  **Build the Go application:**
    ```bash
    go mod tidy
    go build -o ascii_player.exe .
    ```

---

## 🎮 Usage

Launch the interactive player from your terminal:

```bash
./ascii_player.exe
```

### The Workflow

1.  **Select Media**: Use the arrow keys in the interactive file picker to locate your video (`.mp4`, `.avi`, `.mkv`, etc.).
2.  **Conversion**: ASCII Zen will seamlessly spawn a background process to process the video, displaying a real-time progress indicator.
3.  **Playback or Export**: Once complete, you will be prompted to either play the video immediately or compile it into a standalone executable.

### Playback Controls

| Key Binding | Action |
| :--- | :--- |
| `<Space>` | Play / Pause |
| `←` / `→` | Seek backward / forward by 5 seconds |
| `Esc` | Return to the main menu (from playback) or File Picker (from menu) |
| `q` | Quit the application |
| `Ctrl+C` | Force quit |

---

## 🏗️ Architecture

ASCII Zen utilizes a hybrid architecture to maximize performance and developer ergonomics.

1.  **Python (`convert.py`)**: Acts as a high-throughput video processing engine. It reads the raw video file using `cv2`, downscales the frames, and uses `NumPy` to map pixel intensities to a predefined ASCII character set. The resulting raw byte stream is piped directly to `stdout`.
2.  **Go (`main.go`)**: Acts as the orchestrator and UI layer. It manages the `convert.py` subprocess, buffers the incoming frame stream asynchronously, and renders the interactive TUI using Bubble Tea.
3.  **Standalone Compiler**: When exporting, Go temporarily modifies `data.go` to use `//go:embed` directives, effectively packaging the compressed ASCII frame data directly into a newly compiled binary.

---

## 🤝 Contributing

We welcome contributions to ASCII Zen! Please see our [Contributing Guidelines](CONTRIBUTING.md) for more details on how to get started, report bugs, or request features.

---

## 📄 License

Distributed under the MIT License. See [`LICENSE`](LICENSE) for more information.

<div align="center">
  <p>Built by <a href="https://github.com/githubuser2777">githubuser2777</a></p>
</div>
