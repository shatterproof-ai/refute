# LSP Landscape

This is the shared reference for language-server and refactoring-engine
research. It informs backend expansion strategy; the release support level for
shipped behavior remains in [`support-matrix.md`](support-matrix.md).

The [LSP specification](https://github.com/Microsoft/language-server-protocol/blob/gh-pages/_specifications/lsp/3.17/specification.md)
standardizes `textDocument/rename`, `workspace/symbol`, `WorkspaceEdit`, and
`textDocument/codeAction`, but it does not standardize a rich refactoring
taxonomy. Code actions may return edits directly, require `codeAction/resolve`,
or return commands that must be executed by the server. `refute` can normalize
transport mechanics, but still needs language-specific operation mapping and
backend-specific escape hatches.

| Language | Main substrate | `refute` posture |
| --- | --- | --- |
| Go | [`gopls`](https://go.dev/gopls/features/transformation) | Use `gopls` as the reference LSP-backed implementation. It documents rename plus extract function/method/variable, extract declarations to a new file, inline call, and parameter movement actions. Keep operation-level tests because extract and some code actions have known caveats. |
| TypeScript/JavaScript | TypeScript language service via [`typescript-language-server`](https://github.com/typescript-language-server/typescript-language-server), tsserver, and direct ts-morph APIs | Do not rely on generic LSP alone. Use the common rename/edit path where possible, but keep TypeScript-specific adapter paths for extract, move, organize imports, file rename, project-wide edits, refactor arguments, and other operations that do not map cleanly to generic LSP. |
| Python | Pyright, Pylance, Basedpyright, and rope | Treat Python as a dedicated track. Pyright is useful for open-source type analysis and LSP rename/discovery. Pylance has richer closed-source editor refactorings. Basedpyright is worth evaluating as an open-source language-server alternative. Rope is the strongest fit for deterministic library-driven rename, extract, inline, move, and change-signature refactorings. |
| Rust | [`rust-analyzer`](https://rust-analyzer.github.io/book/assists.html) | Use rust-analyzer for rename, symbol discovery, assists, and code actions. Action names and edit shapes are rust-analyzer-specific, so support claims need pinned tests around each operation. |
| Java | [`eclipse.jdt.ls`](https://github.com/eclipse-jdtls/eclipse.jdt.ls) plus OpenRewrite | Keep both. JDT LS fits editor-like LSP operations; OpenRewrite fits recipe-driven, large-scale transformations that exceed normal editor refactorings. |
| Kotlin | [`kotlin-lsp`](https://github.com/Kotlin/kotlin-lsp) and IntelliJ-derived tooling | Treat as exploratory until official Kotlin LSP behavior is stable enough for pinned fixture tests. |
| C/C++ | [`clangd`](https://clangd.llvm.org/features) | Start with rename only if this language is added. Document limitations around templates, macros, overridden methods, comments, and stale indexes before exposing support. |
| C# | Roslyn-backed LSPs, C# Dev Kit/Roslyn, and `csharp-ls` | Choose a Roslyn-backed server story before implementation. Expect non-standard extension points, and avoid treating C# as a generic-LSP-only target. |
| PHP/Ruby | Intelephense and Solargraph | Treat as opportunistic later targets. Document server licensing and capability limits first; Intelephense gates some features behind premium licensing, while Solargraph's code actions remain limited. |
