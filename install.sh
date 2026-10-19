#!/bin/sh
# Установка b24 и скилла bitrix24 для Claude Code на macOS.
#   curl -fsSL https://raw.githubusercontent.com/archiZubarev/b24/main/install.sh | sh
set -eu

REPO="archiZubarev/b24"
BASE="${B24_BASE:-https://github.com/$REPO/releases/latest/download}"
BIN_DIR="${B24_BIN_DIR:-$HOME/.local/bin}"
SKILLS="$HOME/.claude/skills"

if [ "$(uname -s)" != "Darwin" ]; then
	echo "Этот установщик — для macOS. На Windows: install.ps1 (см. README)." >&2
	exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Скачиваю b24 и скилл…"
curl -fsSL "$BASE/b24-darwin-universal" -o "$tmp/b24"
curl -fsSL "$BASE/bitrix24-skill.zip" -o "$tmp/skill.zip"
curl -fsSL "$BASE/checksums.txt" -o "$tmp/checksums.txt"

check() {
	want=$(grep " $2\$" "$tmp/checksums.txt" | cut -d' ' -f1)
	got=$(shasum -a 256 "$1" | cut -d' ' -f1)
	if [ -z "$want" ] || [ "$want" != "$got" ]; then
		echo "Контрольная сумма $2 не сошлась — файл повреждён или подменён, установка остановлена." >&2
		exit 1
	fi
}
check "$tmp/b24" b24-darwin-universal
check "$tmp/skill.zip" bitrix24-skill.zip

mkdir -p "$BIN_DIR"
install -m 0755 "$tmp/b24" "$BIN_DIR/b24"
echo "✓ b24 → $BIN_DIR/b24"

mkdir -p "$SKILLS"
rm -rf "$SKILLS/bitrix24"
unzip -q "$tmp/skill.zip" -d "$SKILLS"
echo "✓ скилл → $SKILLS/bitrix24"

case ":$PATH:" in
*":$BIN_DIR:"*) ;;
*)
	profile="$HOME/.zprofile"
	[ -n "${SHELL:-}" ] && [ "$(basename "$SHELL")" = "bash" ] && profile="$HOME/.bash_profile"
	if ! grep -qs "$BIN_DIR" "$profile"; then
		printf '\n# b24\nexport PATH="%s:$PATH"\n' "$BIN_DIR" >>"$profile"
		echo "✓ $BIN_DIR добавлен в PATH ($profile) — откройте новое окно терминала"
	fi
	;;
esac

if [ -z "${B24_SKIP_SETUP:-}" ]; then
	"$BIN_DIR/b24" setup || echo "MCP не подключён: выполните позже b24 setup"
fi

cat <<'EOF'

Дальше:
  1. В Битриксе: Приложения → Разработчикам → Готовые сценарии → Другое → Входящий вебхук.
     Отметьте нужные права, сохраните, скопируйте ссылку «Вебхук для вызова rest api».
  2. В терминале: b24 add work --clipboard   (ссылку в чат с Claude не вставлять)
  3. Откройте новый разговор с Claude — он увидит портал через MCP.
EOF
