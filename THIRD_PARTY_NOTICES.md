# Third-party notices

tuimessenger's own license is in `LICENSE`. These are the licenses of third-party works shipped inside tuimessenger
binaries in a form other than Go source modules (Go modules keep their own license files).

## terminal-notifier

- What: `assets/terminal-notifier.zip`, embedded in macOS builds and installed as `tuimessenger.app`, which shows
  tuimessenger's notifications under its own name and icon.
- Source: https://github.com/julienXX/terminal-notifier, release 3.1.0, built by `scripts/build-notifier.sh`
  (universal arm64 + x86_64). Apple's `Terminal.icns` is removed before packaging; tuimessenger uses its own icon.
- License (from the project's `LICENSE.md`; the Terminal.icns exception doesn't apply, as it isn't shipped):

> All the works are available under the MIT license. Except for ‘Terminal.icns’, which is a copy of Apple’s Terminal.app icon and as such is copyright of Apple.
> 
> Copyright (C) 2012-2016 Eloy Durán eloy.de.enige@gmail.com, Julien Blanchard julien@sideburns.eu
> 
> Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:
> 
> The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.
> 
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

## Network notification icons

- What: `assets/notifiers/*.icns`, embedded in macOS builds and installed as `WhatsApp.app`, `Telegram.app`,
  `Slack.app` and `Instagram.app`, so each network's notifications show its icon.
- Made by: the WhatsApp, Telegram, Slack and Instagram glyphs of [Font Awesome Free](https://fontawesome.com) (as
  included in [Nerd Fonts](https://www.nerdfonts.com)), recoloured and placed on tuimessenger's dark tile.
- License: Font Awesome Free icons are under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/)
  (© Fonticons, Inc.); these are modified versions.
- The logos are trademarks of their owners (WhatsApp and Instagram: Meta Platforms, Inc.; Telegram: Telegram FZ-LLC;
  Slack: Slack Technologies, LLC). They're used only to show which network a notification is from; tuimessenger
  isn't affiliated with or endorsed by them.

## opencode themes

- What: `internal/tui/themes/opencode/*.json`, opencode's built-in TUI themes, embedded in every build and offered as
  tuimessenger's themes (`theme:` in tuimessenger.yaml, Ctrl+p → Theme). tuimessenger reads their colour slots and
  maps them onto its own colours.
- Source: https://github.com/anomalyco/opencode, `packages/tui/src/theme/assets/`, commit 0bd8cee7c2c77d04cad50dace81520c34c805c3d, downloaded unchanged by
  `scripts/update-opencode-themes.sh`.
- License (opencode's `LICENSE`, also at `internal/tui/themes/opencode/LICENSE`):

> MIT License
>
> Copyright (c) 2025 opencode
>
> Permission is hereby granted, free of charge, to any person obtaining a copy
> of this software and associated documentation files (the "Software"), to deal
> in the Software without restriction, including without limitation the rights
> to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
> copies of the Software, and to permit persons to whom the Software is
> furnished to do so, subject to the following conditions:
>
> The above copyright notice and this permission notice shall be included in all
> copies or substantial portions of the Software.
>
> THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
> IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
> FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
> AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
> LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
> OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
> SOFTWARE.
