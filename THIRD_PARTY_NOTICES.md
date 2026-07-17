# Third-party notices

Callee embeds Markdown component content from
[Microsoft PromptKit](https://github.com/microsoft/PromptKit) through the pinned
[PromptKitty](https://github.com/baldaworks/promptkitty) Go dependency.
PromptKit is distributed under the MIT License; its original SPDX and copyright
headers remain in the embedded component files.

Callee uses the BM25 implementation from
[vecgo](https://github.com/hupe1980/vecgo), version `v0.0.15`, through
PromptKitty. vecgo is distributed under the Apache License 2.0. Its license text
is stored in `third_party/vecgo/LICENSE` and appended to every statically linked
npm package license during release staging.
