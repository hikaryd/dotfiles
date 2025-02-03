#!/bin/bash
# shellcheck source=/dev/null

start_bar() {
	# Navigate to the $HOME/dotfiles/bar directory
	cd "$HOME/dotfiles/bar" || {
		echo -e "\033[31mDirectory $HOME/dotfiles/bar does not exist.\033[0m\n"
		exit 1
	}

	# Check if the virtual environment exists, if not, create it
	if [ ! -d .venv ]; then
		echo -e "\033[32m  venv does not exist. Creating venv...\033[0m\n"
		python3 -m venv .venv

		if [ $? -ne 0 ]; then
			echo -e "\033[31m  Failed to create virtual environment. Exiting...\033[0m\n"
			exit 1
		fi

		echo -e "\033[32m  Installing python dependencies, brace yourself.\033[0m\n"
		.venv/bin/python -m pip install -r requirements.txt

		if [ $? -ne 0 ]; then
			echo -e "\033[31mFailed to install packages from requirements.txt. Exiting...\033[0m\n"
			exit 1
		fi
		echo -e "\033[32m  All done, starting bar.\033[0m\n"
	else
		echo -e "\033[32m  Using existing venv.\033[0m\n"
	fi

	cat <<EOF


 __ __  __ __  ___      ___  ____   ____  ____     ___  _
|  |  ||  |  ||   \    /  _]|    \ /    ||    \   /  _]| |
|  |  ||  |  ||    \  /  [_ |  o  )  o  ||  _  | /  [_ | |
|  _  ||  ~  ||  D  ||    _]|   _/|     ||  |  ||    _]| |___
|  |  ||___, ||     ||   [_ |  |  |  _  ||  |  ||   [_ |     |
|  |  ||     ||     ||     ||  |  |  |  ||  |  ||     ||     |
|__|__||____/ |_____||_____||__|  |__|__||__|__||_____||_____|


EOF
	echo -e "\e[32mUsing python:\e[0m \e[34m$(which python)\e[0m\n"

	pkill dunst
	pkill mako
	.venv/bin/python ./main.py
	deactivate
}

start_bar
