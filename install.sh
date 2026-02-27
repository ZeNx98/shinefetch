#!/usr/bin/env bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}==>${NC} Installing ${GREEN}shinefetch${NC}..."

# Function to ask for permission
ask_permission() {
    local prompt="$1"
    read -p "$(echo -e "${YELLOW}??${NC} $prompt [y/N]: ")" -n 1 -r
    echo
    [[ $REPLY =~ ^[Yy]$ ]]
}

# 1. Detect Distro and Install Dependencies
install_deps() {
    local missing_sys_deps=()
    local install_pokeget=false

    command -v go >/dev/null 2>&1 || missing_sys_deps+=("go")
    command -v fastfetch >/dev/null 2>&1 || missing_sys_deps+=("fastfetch")
    
    if ! command -v pokeget >/dev/null 2>&1; then
        install_pokeget=true
        command -v cargo >/dev/null 2>&1 || missing_sys_deps+=("cargo")
    fi

    if [ ${#missing_sys_deps[@]} -eq 0 ] && [ "$install_pokeget" = false ]; then
        echo -e "${GREEN}==>${NC} All dependencies (go, fastfetch, pokeget) are already installed."
        return 0
    fi

    if [ ${#missing_sys_deps[@]} -gt 0 ]; then
        echo -e "${BLUE}==>${NC} Missing system dependencies: ${YELLOW}${missing_sys_deps[*]}${NC}"
        if ! ask_permission "Would you like to install missing system dependencies?"; then
            echo -e "${RED}Error:${NC} System dependencies (go, fastfetch) are required."
            exit 1
        fi

        # Detect Package Manager
        if [ -f /etc/os-release ]; then
            . /etc/os-release
            case $ID in
                arch|manjaro|cachyos)
                    echo -e "${BLUE}==>${NC} Syncing package database..."
                    sudo pacman -Sy
                    sudo pacman -S --needed --noconfirm "${missing_sys_deps[@]}" ;;
                ubuntu|debian|pop|kali|neon|pureos)
                    echo -e "${BLUE}==>${NC} Updating package list..."
                    sudo apt update
                    sudo apt install -y "${missing_sys_deps[@]}" ;;
                fedora)
                    echo -e "${BLUE}==>${NC} Refreshing dnf cache..."
                    sudo dnf makecache
                    sudo dnf install -y "${missing_sys_deps[@]}" ;;
                opensuse*|suse)
                    echo -e "${BLUE}==>${NC} Refreshing zypper repositories..."
                    sudo zypper refresh
                    sudo zypper install -y "${missing_sys_deps[@]}" ;;
                voider)
                    echo -e "${BLUE}==>${NC} Syncing xbps repository..."
                    sudo xbps-install -S
                    sudo xbps-install -y "${missing_sys_deps[@]}" ;;
                *)
                    echo -e "${RED}Error:${NC} Manual installation of ${YELLOW}${missing_sys_deps[*]}${NC} required."
                    exit 1 ;;
            esac
        fi
    fi

    if [ "$install_pokeget" = true ]; then
        echo -e "${BLUE}==>${NC} Installing ${GREEN}pokeget${NC} via cargo..."
        cargo install pokeget || { echo -e "${RED}Error:${NC} Cargo install failed."; exit 1; }
        export PATH="$HOME/.cargo/bin:$PATH"
    fi
}

# Run dependency installation
install_deps

# 2. Build the binary
echo -e "${BLUE}==>${NC} Building binary..."
go build -o shinefetch . || { echo -e "${RED}Error:${NC} Build failed."; exit 1; }

# 3. Install binary and config
mkdir -p "$HOME/.local/bin"
cp shinefetch "$HOME/.local/bin/"
echo -e "${BLUE}==>${NC} Binary installed to ${GREEN}~/.local/bin/shinefetch${NC}"

echo -e "${BLUE}==>${NC} Installing default configuration..."
mkdir -p "$HOME/.config/shinefetch"

# Install fastfetch.jsonc
if [ ! -f "$HOME/.config/shinefetch/fastfetch.jsonc" ]; then
    cp fastfetch.jsonc "$HOME/.config/shinefetch/"
    echo -e "${GREEN}==>${NC} Fastfetch config installed to ~/.config/shinefetch/fastfetch.jsonc"
else
    echo -e "${BLUE}==>${NC} Fastfetch config already exists, skipping."
fi

# Install settings.jsonc
if [ ! -f "$HOME/.config/shinefetch/settings.jsonc" ]; then
    cp settings.jsonc "$HOME/.config/shinefetch/"
    echo -e "${GREEN}==>${NC} Behavior config installed to ~/.config/shinefetch/settings.jsonc"
else
    echo -e "${BLUE}==>${NC} Behavior config already exists, skipping."
fi

# 4. Prompt for Shell Integration
# Detect Shell
CURRENT_SHELL=$(basename "$SHELL")
SHELL_CONFIG=""

case "$CURRENT_SHELL" in
    zsh)  SHELL_CONFIG="$HOME/.zshrc" ;;
    bash) SHELL_CONFIG="$HOME/.bashrc" ;;
    fish) SHELL_CONFIG="$HOME/.config/fish/config.fish" ;;
    *) 
        # Fallback/Guessing
        if [ -f "$HOME/.zshrc" ]; then SHELL_CONFIG="$HOME/.zshrc"
        elif [ -f "$HOME/.bashrc" ]; then SHELL_CONFIG="$HOME/.bashrc"
        fi
        ;;
esac

if [ -n "$SHELL_CONFIG" ]; then
    echo -e "${BLUE}==>${NC} Setting up shell integration in $SHELL_CONFIG..."
    mkdir -p "$(dirname "$SHELL_CONFIG")"
    touch "$SHELL_CONFIG"
    
    # 1. Add PATHs if missing
    PATHS_TO_ADD=("$HOME/.local/bin" "$HOME/.cargo/bin")
    for p in "${PATHS_TO_ADD[@]}"; do
        if ! grep -q "export PATH=.*$p" "$SHELL_CONFIG"; then
            echo -e "\n# Added by Shinefetch\nexport PATH=\"\$PATH:$p\"" >> "$SHELL_CONFIG"
            echo -e "${GREEN}==>${NC} Added $p to PATH in $SHELL_CONFIG"
        fi
    done

    # 2. Add Alias
    ALIAS_CMD="alias pfetch='shinefetch'"
    if [[ "$CURRENT_SHELL" == "fish" ]]; then
        # Fish uses different syntax for path and alias
        if ! grep -q "fish_add_path $HOME/.local/bin" "$SHELL_CONFIG"; then
            echo "fish_add_path $HOME/.local/bin" >> "$SHELL_CONFIG"
        fi
        if ! grep -q "fish_add_path $HOME/.cargo/bin" "$SHELL_CONFIG"; then
            echo "fish_add_path $HOME/.cargo/bin" >> "$SHELL_CONFIG"
        fi
        ALIAS_CMD="alias pfetch='shinefetch'"
    fi

    if ! grep -q "alias pfetch=" "$SHELL_CONFIG" && ! grep -q "alias pfetch " "$SHELL_CONFIG"; then
        echo -e "\n# Shinefetch Alias\n$ALIAS_CMD" >> "$SHELL_CONFIG"
        echo -e "${GREEN}==>${NC} Alias added to $SHELL_CONFIG."
    else
        echo -e "${BLUE}==>${NC} Alias pfetch already exists in $SHELL_CONFIG."
    fi

    # 3. Auto-run on startup
    if ask_permission "Would you like to auto-run shinefetch every time you open a terminal?"; then
        if ! grep -Fxq "shinefetch" "$SHELL_CONFIG" && ! grep -Fxq "pfetch" "$SHELL_CONFIG"; then
            echo -e "\n# Auto-run Shinefetch\nshinefetch" >> "$SHELL_CONFIG"
            echo -e "${GREEN}==>${NC} Auto-run added to $SHELL_CONFIG."
        else
            echo -e "${BLUE}==>${NC} Auto-run already configured in $SHELL_CONFIG."
        fi
    fi
else
    echo -e "${YELLOW}Notice:${NC} Could not detect shell config file. Please add ~/.local/bin and ~/.cargo/bin to your PATH manually."
fi

echo -e "\n${GREEN}Installation complete!${NC}"
echo -e "You can now run ${GREEN}shinefetch${NC} or ${GREEN}pfetch${NC} (if alias was added)."
