---
name: page-builder
description: Dựng trang web chạy được từ blueprint và nội dung: một file HTML mở thẳng bằng trình duyệt, menu dính, Hero, lợi ích, form liên hệ, hợp mobile.
---

# Page builder

Khi dựng trang:

1. Đọc blueprint và nội dung đã duyệt; không tự đổi chữ, màu, font. Thiếu thì dùng chỗ giữ chỗ có ghi chú, không bịa nội dung.
2. Nếu project đã có framework (Nuxt, Vue, React…), dựng theo quy ước và component có sẵn. Chỉ làm một file `index.html` (CSS, JS nội tuyến, không build) khi người yêu cầu hoặc project chưa có gì.
3. Bắt buộc: menu dính khi cuộn, Hero có CTA chính, khối lợi ích, form liên hệ có nhãn, kiểm tra nhập và trạng thái gửi/lỗi, footer.
4. Mobile trước: chạy tốt ở 375px, menu thu gọn, chữ ≥ 16px, vùng bấm ≥ 44px. HTML ngữ nghĩa, có `alt`, `lang`, title và meta description.
5. Form không gửi đi đâu nếu chưa có địa chỉ nhận thật; ghi rõ chỗ cần nối.

Trước khi báo xong: mở trang thật, chụp ảnh desktop và mobile, xem console không lỗi. Báo file đã tạo, ảnh và những chỗ còn giữ chỗ.
