---
title: pensa
description: A fast enough Python package and project manager, written in Go. Single binary, UNIX-style CLI, independent open source.
layout: hextra-home
---

{{< hextra/hero-badge >}}
  <span>v0.1.0</span>
{{< /hextra/hero-badge >}}

<div class="hx:mt-6 hx:mb-6">
{{< hextra/hero-headline >}}
  A fast enough Python package manager,&nbsp;<br class="hx:sm:block hx:hidden" />written in Go.
{{< /hextra/hero-headline >}}
</div>

<div class="hx:mb-12">
{{< hextra/hero-subtitle >}}
  Single binary. UNIX-style CLI. Independent open source.
{{< /hextra/hero-subtitle >}}
</div>

<div class="hx:mb-6">
{{< hextra/hero-button text="Get Started" link="docs/getting-started" >}}
</div>

```bash
brew install pensa-sh/tap/pensa
```

{{< hextra/feature-grid >}}
  {{< hextra/feature-card
    title="Single binary"
    subtitle="No runtime dependencies. Download and run."
  >}}
  {{< hextra/feature-card
    title="Reads your project files"
    subtitle="Works with pyproject.toml in both PEP 621 and Poetry formats. Reads pensa.lock, uv.lock, and poetry.lock."
  >}}
  {{< hextra/feature-card
    title="UNIX-style CLI"
    subtitle="Focused commands that do one thing. Composable with other tools. Designed around clig.dev."
  >}}
{{< /hextra/feature-grid >}}
