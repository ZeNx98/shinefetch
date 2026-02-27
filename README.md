# Shinefetch

<div align="center">
  <img src="imgs/normal.png" alt="Shinefetch Normal Preview" width="45%" height="auto">
  <img src="imgs/shiny.png" alt="Shinefetch Shiny Preview" width="45.3%" height="auto">
</div>

Shinefetch is an advanced Pokemon themed fetch tool. It uses fastfetch as a base to display system information alongside a random pokemon sprite, dynamically extracting UI colors directly from the Pokemon.

## Features

1. Displays random Pokemon sprites alongside fastfetch system information.
2. Features rare shiny encounters with animated breathing UI borders.
3. Extracts dominant colors from sprites for dynamic UI theming.
4. Includes a persistent tracker for shiny Pokemon encounters.
5. Supports highly configurable borders, spacing, and shiny encounter rates.
6. Content adapt scaling and stays in the center of the terminal.

## Dependencies

The following software is required to run Shinefetch.

1. go compiler
2. fastfetch
3. pokeget
4. cargo (used by the installer if pokeget is missing)

Note: The installation script will automatically detect and download these missing dependencies for your system.

## Installation

You can install Shinefetch by cloning the repository and running the installation script.

1. Run the following commands to install Shinefetch

```bash
git clone https://github.com/ZeNx98/shinefetch.git
cd shinefetch
chmod +x install.sh
./install.sh
```

The installation script automatically detects your Linux distribution, installs required dependencies, compiles the binary, and prompts you to configure your shell.

## Configuration

Settings are located in your ~/.config/shinefetch folder. Edit the configuration file to customize the application.

1. Adjust the chance of finding a shiny Pokemon (default is 1/100).
2. Toggle interactive border animations.
3. Switch border styles between rounded, sharp, double, and heavy.
4. Override the trainer name displayed in the stats.
5. Adjust the gap between the sprite and the system information box.
6. Change the alignment of the Pokedex information box.
7. Print and exit mode for static configuration in bashrc.
