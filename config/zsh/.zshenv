# rustup PATH, inlined to avoid sourcing ~/.cargo/env for every zsh process.
case ":${PATH}:" in
  *:"$HOME/.cargo/bin":*) ;;
  *) export PATH="$HOME/.cargo/bin:$PATH" ;;
esac
. "$HOME/.cargo/env"
