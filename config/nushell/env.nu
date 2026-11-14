$env.PATH = ($env.PATH | prepend ($nu.home-dir | path join ".local/bin") | uniq)
