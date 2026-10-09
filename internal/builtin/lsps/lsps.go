package lsps

import (
	"github.com/peggco/pegg/internal/core/config"
	"github.com/peggco/pegg/internal/lsp"
)

func init() {
	lsp.Register("astro", config.LanguageServerConfig{
		Command:     "astro-ls",
		Args:        []string{"--stdio"},
		FileTypes:   []string{"astro"},
		RootMarkers: []string{"astro.config.mjs", "astro.config.mts"},
		Install:     "npm install -g @astrojs/language-server",
	})
	lsp.Register("bash", config.LanguageServerConfig{
		Command:     "bash-language-server",
		Args:        []string{"start"},
		FileTypes:   []string{"sh", "bash", "zsh", "ksh"},
		RootMarkers: []string{".bashrc", ".zshrc"},
		Install:     "npm install -g bash-language-server",
	})
	lsp.Register("clangd", config.LanguageServerConfig{
		Command:     "clangd",
		Args:        []string{"--background-index"},
		FileTypes:   []string{"c", "cpp", "cc", "cxx", "c++", "h", "hpp", "hh", "hxx", "h++"},
		RootMarkers: []string{"compile_commands.json", "CMakeLists.txt", "Makefile"},
		Install:     "(apt-get install -y clangd 2>/dev/null) || (brew install clangd 2>/dev/null) || (nix-env -iA nixpkgs.clang-tools 2>/dev/null) || (snap install clangd --classic 2>/dev/null)",
	})
	lsp.Register("csharp", config.LanguageServerConfig{
		Command:     "csharp-ls",
		Args:        []string{},
		FileTypes:   []string{"cs", "csx"},
		RootMarkers: []string{".csproj", "*.sln"},
		Install:     "dotnet tool install -g csharp-ls",
	})
	lsp.Register("clojure-lsp", config.LanguageServerConfig{
		Command:     "clojure-lsp",
		Args:        []string{},
		FileTypes:   []string{"clj", "cljs", "cljc", "edn"},
		RootMarkers: []string{"deps.edn", "project.clj", "bb.edn"},
		Install:     "(brew install clojure-lsp 2>/dev/null) || (curl -sL -o \"$HOME/.pegg/lsps/clojure-lsp.zip\" https://github.com/clojure-lsp/clojure-lsp/releases/latest/download/clojure-lsp-native-static-linux-amd64.zip)",
	})
	lsp.Register("dart", config.LanguageServerConfig{
		Command:     "dart",
		Args:        []string{"language-server", "--protocol=lsp"},
		FileTypes:   []string{"dart"},
		RootMarkers: []string{"pubspec.yaml"},
		Install:     `curl -sL -o /tmp/dartsdk.zip https://storage.googleapis.com/dart-archive/channels/stable/release/latest/sdk/dartsdk-linux-x64-release.zip && unzip -q -o /tmp/dartsdk.zip -d "$HOME/.pegg" && ln -sf "$HOME/.pegg/dart-sdk/bin/dart" "$HOME/.pegg/lsps/dart"`,
	})
	lsp.Register("deno", config.LanguageServerConfig{
		Command:     "deno",
		Args:        []string{"lsp"},
		FileTypes:   []string{"ts", "tsx", "js", "jsx", "mjs"},
		RootMarkers: []string{"deno.json", "deno.jsonc"},
		Install:     "curl -fsSL https://deno.land/install.sh | sh",
	})
	lsp.Register("elixir-ls", config.LanguageServerConfig{
		Command:     "elixir-ls",
		Args:        []string{},
		FileTypes:   []string{"ex", "exs"},
		RootMarkers: []string{"mix.exs"},
		Install:     "(brew install elixir-ls 2>/dev/null) || (git clone --depth 1 https://github.com/elixir-lsp/elixir-lsp \"$HOME/.pegg/elixir-ls\" && cd \"$HOME/.pegg/elixir-ls\" && mix deps.get && mix compile && mix escript.build && cp elixir-ls \"$HOME/.pegg/lsps/elixir-ls\")",
	})
	lsp.Register("eslint", config.LanguageServerConfig{
		Command:     "vscode-eslint-language-server",
		Args:        []string{"--stdio"},
		FileTypes:   []string{"ts", "tsx", "js", "jsx", "mjs", "cjs", "mts", "cts", "vue"},
		RootMarkers: []string{".eslintrc", ".eslintrc.json", ".eslintrc.js", "eslint.config.js", "eslint.config.mjs"},
		Install:     "npm install -g vscode-langservers-extracted",
	})
	lsp.Register("fsharp", config.LanguageServerConfig{
		Command:     "fsautocomplete",
		Args:        []string{"--adaptive-lsp-server-enabled"},
		FileTypes:   []string{"fs", "fsi", "fsx", "fsscript"},
		RootMarkers: []string{".fsproj", "*.sln"},
		Install:     "dotnet tool install -g fsautocomplete",
	})
	lsp.Register("gleam", config.LanguageServerConfig{
		Command:     "gleam",
		Args:        []string{"lsp"},
		FileTypes:   []string{"gleam"},
		RootMarkers: []string{"gleam.toml"},
		Install:     `V=$(curl -s https://api.github.com/repos/gleam-lang/gleam/releases/latest | grep -o '"tag_name":"[^"]*"' | sed 's/.*:"//;s/"//') && curl -sL -o "$HOME/.pegg/lsps/gleam.tar.gz" "https://github.com/gleam-lang/gleam/releases/download/$V/gleam-x86_64-unknown-linux-musl.tar.gz"`,
	})
	lsp.Register("gopls", config.LanguageServerConfig{
		Command:     "gopls",
		Args:        []string{"serve"},
		FileTypes:   []string{"go"},
		RootMarkers: []string{"go.mod"},
		Install:     "go install golang.org/x/tools/gopls@latest",
	})
	lsp.Register("hls", config.LanguageServerConfig{
		Command:     "haskell-language-server-wrapper",
		Args:        []string{"--lsp"},
		FileTypes:   []string{"hs", "lhs"},
		RootMarkers: []string{"*.cabal", "stack.yaml", "cabal.project"},
		Install:     "(ghcup install hls --latest 2>/dev/null) || (brew install haskell-language-server 2>/dev/null) || (stack install haskell-language-server --install-ghc 2>/dev/null)",
	})
	lsp.Register("jdtls", config.LanguageServerConfig{
		Command:     "jdtls",
		Args:        []string{},
		FileTypes:   []string{"java"},
		RootMarkers: []string{"pom.xml", "build.gradle", "settings.gradle", ".project"},
		Install:     `curl -sL -o /tmp/jdtls.tar.gz https://download.eclipse.org/jdtls/snapshots/jdt-language-server-latest.tar.gz && mkdir -p "$HOME/.pegg/jdtls" && tar -xzf /tmp/jdtls.tar.gz -C "$HOME/.pegg/jdtls" && cp "$HOME/.pegg/jdtls/bin/jdtls" "$HOME/.pegg/lsps/jdtls"`,
	})
	lsp.Register("julials", config.LanguageServerConfig{
		Command:     "julia",
		Args:        []string{"--startup-file=no", "--history-file=no", "-e", "using LanguageServer; LanguageServer.runserver()"},
		FileTypes:   []string{"jl"},
		RootMarkers: []string{"Project.toml"},
		Install:     `curl -fsSL https://install.julialang.org | sh -s -- --yes && "$HOME/.juliaup/bin/julia" -e 'using Pkg; Pkg.add("LanguageServer")'`,
	})
	lsp.Register("kotlin-ls", config.LanguageServerConfig{
		Command:     "kotlin-language-server",
		Args:        []string{},
		FileTypes:   []string{"kt", "kts"},
		RootMarkers: []string{"build.gradle.kts", "build.gradle", "pom.xml", "settings.gradle"},
		Install:     "brew install kotlin-language-server",
	})
	lsp.Register("lua-ls", config.LanguageServerConfig{
		Command:     "lua-language-server",
		Args:        []string{},
		FileTypes:   []string{"lua"},
		RootMarkers: []string{".luarc.json", ".luarc.jsonc"},
		Install:     "brew install lua-language-server",
	})
	lsp.Register("nixd", config.LanguageServerConfig{
		Command:     "nixd",
		Args:        []string{},
		FileTypes:   []string{"nix"},
		RootMarkers: []string{"flake.nix", "shell.nix", "default.nix"},
		Install:     "(nix profile install nixpkgs#nixd 2>/dev/null) || (nix-env -iA nixpkgs.nixd 2>/dev/null) || (git clone --depth 1 https://github.com/nix-community/nixd \"$HOME/.pegg/nixd\" && cmake -S \"$HOME/.pegg/nixd\" -B \"$HOME/.pegg/nixd/build\" -DCMAKE_BUILD_TYPE=Release && cmake --build \"$HOME/.pegg/nixd/build\" -j4 && cp \"$HOME/.pegg/nixd/build/bin/nixd\" \"$HOME/.pegg/lsps/nixd\")",
	})
	lsp.Register("ocaml-lsp", config.LanguageServerConfig{
		Command:     "ocamllsp",
		Args:        []string{},
		FileTypes:   []string{"ml", "mli"},
		RootMarkers: []string{"dune-project", "*.opam"},
		Install:     `opam install -y ocaml-lsp-server && cp "$(opam var bin)/ocamllsp" "$HOME/.pegg/lsps/ocamllsp"`,
	})
	lsp.Register("oxlint", config.LanguageServerConfig{
		Command:     "oxlint",
		Args:        []string{"--linter=lsp", "--stdio"},
		FileTypes:   []string{"ts", "tsx", "js", "jsx", "mjs", "cjs", "mts", "cts", "vue", "astro", "svelte"},
		RootMarkers: []string{"oxlint.json", ".oxlintrc.json", "package.json"},
		Install:     "npm install -g oxlint",
	})
	lsp.Register("php", config.LanguageServerConfig{
		Command:     "intelephense",
		Args:        []string{"--stdio"},
		FileTypes:   []string{"php"},
		RootMarkers: []string{"composer.json", "phpunit.xml"},
		Install:     "npm install -g intelephense",
	})
	lsp.Register("prisma", config.LanguageServerConfig{
		Command:     "prisma-language-server",
		Args:        []string{"--stdio"},
		FileTypes:   []string{"prisma"},
		RootMarkers: []string{"schema.prisma"},
		Install:     "npm install -g @prisma/language-server",
	})
	lsp.Register("pyright", config.LanguageServerConfig{
		Command:     "pyright-langserver",
		Args:        []string{"--stdio"},
		FileTypes:   []string{"py", "pyi"},
		RootMarkers: []string{"pyproject.toml", "requirements.txt"},
		Install:     "python3 -m pip install -U pyright",
	})
	lsp.Register("razor", config.LanguageServerConfig{
		Command:     "rzls",
		Args:        []string{},
		FileTypes:   []string{"razor", "cshtml"},
		RootMarkers: []string{".csproj"},
		Install:     "dotnet tool install -g rzls",
	})
	lsp.Register("ruby-lsp", config.LanguageServerConfig{
		Command:     "ruby-lsp",
		Args:        []string{},
		FileTypes:   []string{"rb", "rake", "gemspec", "ru"},
		RootMarkers: []string{"Gemfile", "*.gemspec"},
		Install:     "gem install ruby-lsp",
	})
	lsp.Register("rust", config.LanguageServerConfig{
		Command:     "rust-analyzer",
		Args:        []string{},
		FileTypes:   []string{"rs"},
		RootMarkers: []string{"Cargo.toml"},
		Install:     "rustup component add rust-analyzer",
	})
	lsp.Register("sourcekit-lsp", config.LanguageServerConfig{
		Command:     "sourcekit-lsp",
		Args:        []string{},
		FileTypes:   []string{"swift", "objc", "objcpp"},
		RootMarkers: []string{"Package.swift", ".build"},
		Install:     `curl -sSf https://download.swift.org/swiftly/linux/swiftly-$(uname -m).tar.gz | tar -xz -C "$HOME/.local/bin" && "$HOME/.local/bin/swiftly" init --assume-yes --quiet`,
	})
	lsp.Register("svelte", config.LanguageServerConfig{
		Command:     "svelte-language-server",
		Args:        []string{"--stdio"},
		FileTypes:   []string{"svelte"},
		RootMarkers: []string{"svelte.config.js", "svelte.config.mjs"},
		Install:     "npm install -g svelte-language-server",
	})
	lsp.Register("terraform", config.LanguageServerConfig{
		Command:     "terraform-ls",
		Args:        []string{"serve"},
		FileTypes:   []string{"tf", "tfvars"},
		RootMarkers: []string{".terraform.lock.hcl", "*.tf"},
		Install:     `V=$(curl -sL https://releases.hashicorp.com/terraform-ls/index.json | grep -o '"0\.[0-9]*\.[0-9]*"' | tr -d '"' | sort -uV | tail -1) && curl -sL -o "$HOME/.pegg/lsps/terraform-ls.zip" "https://releases.hashicorp.com/terraform-ls/$V/terraform-ls_${V}_linux_amd64.zip"`,
	})
	lsp.Register("tinymist", config.LanguageServerConfig{
		Command:     "tinymist",
		Args:        []string{"lsp"},
		FileTypes:   []string{"typ", "typc"},
		RootMarkers: []string{"typst.toml"},
		Install:     "curl -sL -o \"$HOME/.pegg/lsps/tinymist.tar.gz\" https://github.com/Myriad-Dreamin/tinymist/releases/latest/download/tinymist-x86_64-unknown-linux-gnu.tar.gz",
	})
	lsp.Register("typescript", config.LanguageServerConfig{
		Command:     "typescript-language-server",
		Args:        []string{"--stdio"},
		FileTypes:   []string{"ts", "tsx", "js", "jsx", "mjs", "cjs", "mts", "cts"},
		RootMarkers: []string{"package.json", "tsconfig.json"},
		Install:     "npm install -g typescript-language-server",
	})
	lsp.Register("vue", config.LanguageServerConfig{
		Command:     "vue-language-server",
		Args:        []string{"--stdio"},
		FileTypes:   []string{"vue"},
		RootMarkers: []string{"vue.config.js", "package.json"},
		Install:     "npm install -g @volar/vue-language-server",
	})
	lsp.Register("yaml-ls", config.LanguageServerConfig{
		Command:     "yaml-language-server",
		Args:        []string{"--stdio"},
		FileTypes:   []string{"yaml", "yml"},
		RootMarkers: []string{".yamllint", "pipeline.yaml"},
		Install:     "npm install -g yaml-language-server",
	})
	lsp.Register("zls", config.LanguageServerConfig{
		Command:     "zls",
		Args:        []string{},
		FileTypes:   []string{"zig", "zon"},
		RootMarkers: []string{"build.zig"},
		Install:     "(brew install zls 2>/dev/null) || (cargo install zls --locked 2>/dev/null) || (curl -sL -o \"$HOME/.pegg/lsps/zls.tar.xz\" https://github.com/zigtools/zls/releases/latest/download/zls-x86_64-linux.tar.xz && tar -xJf \"$HOME/.pegg/lsps/zls.tar.xz\" -C \"$HOME/.pegg/lsps\" zls)",
	})
}
