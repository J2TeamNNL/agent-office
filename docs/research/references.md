# Nguồn tham khảo: công nghệ và skill

Kho bên ngoài dùng để tra cứu khi viết agent, skill hoặc quy trình cho office. Nội dung bên ngoài là **dữ liệu chưa soát**: đọc, viết lại ngắn rồi mới đưa vào `templates/library` (ADR-153, ADR-154).

## Agent

- [agency-agents](https://github.com/msitarzewski/agency-agents) (MIT): ~300 persona Markdown. Đã nhập 173 agent vào `templates/library/agency/` bằng `scripts/sync-agency-library.mjs` (ADR-154).
- [ECC](https://github.com/affaan-m/ECC) (MIT): agent, skill, command, rule, hook cho Claude Code, Codex, Cursor… Bản đã xem: `90606c8` (24/10/2026). `node scripts/sync-ecc-library.mjs [--sha=<commit>] [--dir=<office>/library/skills]`:
  - 71 agent → `templates/library/ecc/` (category theo tên, đa số `code`; `opus/sonnet/haiku` → `strong/balanced/fast`).
  - 311 skill → thư viện máy `<office>/library/skills/`, gắn `source:` link ECC. Skill cùng tên không phải của ECC (của người dùng hoặc antigravity, 25 cái) được giữ. 17 skill kèm `scripts/`, cài vẫn qua kiểm tra an toàn.
  - Không nhập: `commands/` (office chưa có slash command, cái hay thì viết lại thành skill), `rules/` (lấy lẻ, viết lại), `hooks/`, memory, `ecc2/`, dashboard (trùng thứ office đã có, gắn chặt harness Claude Code). `mcp-configs/` chỉ tham khảo.

- [Claude Code Templates](https://github.com/davila7/claude-code-templates) (MIT, web https://aitmpl.com): agent, skill, command, hook, setting, MCP cho Claude Code. Bản đã xem: `aff8306` (26/10/2026). `node scripts/sync-aitmpl-library.mjs [--sha=<commit>] [--dir=<office>/library/skills]` (ADR-216):
  - 408 agent → `templates/library/aitmpl/` (category theo thư mục, mô tả bỏ khối `<example>`; 14 agent trùng tên ở thư mục khác thì lấy cái đầu).
  - Skill → thư viện máy, gắn `source:`, `category` (thư mục gốc đổi sang category thư viện đã biết) và `tags: [<thư mục gốc>]`. Lần đầu: 489 skill mới, 401 cùng tên của nguồn khác được giữ. Bỏ 13 skill có giấy phép không mở (bộ docx/pdf/pptx/xlsx của Anthropic).
  - Không nhập: `commands/`, `hooks/`, `settings/`, `sandbox/`, `mods/`, `loops/` (gắn chặt harness Claude Code). `mcps/` chỉ tham khảo.

## Vòng làm việc của agent (harness)

- [Better Harness](https://github.com/QoderAI/better-harness) (MIT): chấm vòng làm việc theo 5 mặt. Thành quy trình `better-harness` (ADR-181).
- [Harness²](https://github.com/google-research/harness2) (Apache 2.0, arXiv 2610.14224): tự cải thiện vòng làm việc bằng Propose–Probe–Compose và thư viện bài học `cue / lesson / evidence`. Bản đã xem: `1cfe254` (24/10/2026). Đã lấy hai góc INV/DOM, định dạng bài học, quy tắc chọn thành phần vào `better-harness` (ADR-184). Prompt đáng đọc: `harness2/core/prompts.py` (`GENERALIZE`, `_INV_TEMPLATE`, `_DOM_TEMPLATE`, `_KEEP_BASE`).

## Skill

- [antigravity-awesome-skills](https://github.com/sickn33/antigravity-awesome-skills) (code MIT, nội dung CC BY 4.0, cần ghi công khi chép): 2.712 skill dạng `skills/<name>/SKILL.md`, dùng được với Claude Code, Codex, Gemini CLI, Antigravity. Bản đã xem: `71b1c1a` (18/10/2026).
  - Tra cứu: `skills_index.json` (mỗi skill có `category`, `risk`, `source`, `plugin.targets`), `CATALOG.md`, `data/bundles.json` (bộ skill theo vai), web https://aaskills.tech/.
  - Mảng nhiều nhất: security 265, development 226, business 178, devops 166, cloud 147, ai-ml 136, marketing 114.
  - `risk`: safe 1.115, critical 1.317, offensive 146, chưa rõ 134. Cài vào project vẫn qua kiểm tra an toàn (`internal/automation/safety.go`); ưu tiên skill `safe`.
  - **Đã chép cả 2.611 skill (18/10/2026) vào thư viện máy** `<office>/library/skills/` (không vào repo): xem và cài từ trang **Thư viện** → Skills. Skill nhập có `source` trong frontmatter nên bước gợi ý khi tạo project (`setupToolbox`, `internal/api/setup_extras.go`) bỏ qua, tránh prompt phình. Cập nhật (tải về, chép, gắn `category` + `tags` từ `data/catalog.json` vào frontmatter): `node scripts/sync-skill-library.mjs [--sha=<commit>] [--dir=<office>/library/skills]`; skill cùng tên không có `source` (của người dùng) được giữ nguyên. Trang Thư viện lọc theo mảng việc (category gom lại, `SkillArea` trong `internal/automation/library.go`), category, tag, rủi ro.
