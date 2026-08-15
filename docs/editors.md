# Editor integration

`docc lsp` speaks the Language Server Protocol over stdio, so the checks that
gate a build appear while typing.

## NeoVim

`docc lsp` is a dependency-free Language Server Protocol server. It publishes
live diagnostics for Markdown documents using the nearest `.docc/schemas`
directory; `--schema-dir` and `--type` have the same meaning as for `check`.

With NeoVim's built-in LSP client, add this to your `init.lua`:

```lua
vim.api.nvim_create_autocmd("FileType", {
  pattern = "markdown",
  callback = function(args)
    vim.lsp.start({
      name = "docc",
      cmd = { "docc", "lsp" },
      root_dir = vim.fs.root(args.file, { ".docc" }) or vim.fn.getcwd(),
    })
  end,
})
```

The server uses full-document synchronization and reports UTF-16-correct
ranges, including in documents containing non-ASCII text. It currently
provides diagnostics only; completion and code actions remain editor features
for a future release.

Only files whose frontmatter declares the `docc` marker are checked. Plain
markdown and files with unrelated YAML frontmatter get no diagnostics, so
editing regular `.md` files next to docc documents is quiet.
