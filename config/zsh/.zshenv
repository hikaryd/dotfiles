# Cap pytest-xdist's -n auto in every zsh session, including non-interactive ones.
# An explicit environment value or -n N still takes precedence.
export PYTEST_XDIST_AUTO_NUM_WORKERS="${PYTEST_XDIST_AUTO_NUM_WORKERS:-2}"

# rustup PATH, inlined to avoid sourcing ~/.cargo/env for every zsh process.
case ":${PATH}:" in
  *:"$HOME/.cargo/bin":*) ;;
  *) export PATH="$HOME/.cargo/bin:$PATH" ;;
esac
