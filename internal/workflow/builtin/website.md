---
key: website
name: Làm website doanh nghiệp
description: Từ thông tin doanh nghiệp tới website sẵn sàng ra mắt. Lập bản thiết kế tổng thể (khách hàng, 3 đối thủ, hành động chính, các trang), người dùng duyệt, viết nội dung trang chủ, dựng trang, tinh chỉnh thiết kế, rồi kiểm tra trước khi ra mắt. Mỗi bước dùng một skill của thư viện (website-blueprint, homepage-copy, page-builder, design-polish, launch-checklist); bước nào project chưa cài skill thì vẫn chạy, không có hướng dẫn của skill.
input: Doanh nghiệp, khách hàng và điều muốn khách làm trên website
inputs:
  - { key: business, description: "Doanh nghiệp: sản phẩm/dịch vụ, khu vực, điểm khác biệt", required: true }
  - { key: customers, description: "Khách hàng lý tưởng và vấn đề lớn nhất của họ" }
  - { key: action, description: "Hành động chính muốn khách làm (gọi, đặt lịch, mua, để lại form)" }
  - { key: competitors, description: "Đối thủ (tên hoặc website), nếu biết" }
  - { key: proof, description: "Bằng chứng có thật: số liệu, khách hàng, đánh giá, chứng nhận" }
  - { key: brand, description: "Giọng thương hiệu, màu/logo có sẵn" }
outputs:
  - { key: pages, description: "Trang đã dựng và nơi để file", required: true }
  - { key: launch_blockers, description: "Việc phải xong trước khi ra mắt" }
roles:
  - key: chien-luoc
    name: Chiến lược web
    hint: website strategy, conversion, competitor research
    access: analyze
    prefer: { tier: strong }
  - key: noi-dung
    name: Viết nội dung
    hint: conversion copywriting for small business websites
    access: analyze
  - key: dung-trang
    name: Dựng trang
    hint: frontend, responsive HTML/CSS
    access: edit
  - key: thiet-ke
    name: Tinh chỉnh thiết kế
    hint: visual design, typography, spacing, contrast
    access: edit
    differ_from: [dung-trang]
  - key: kiem-tra
    name: Kiểm tra ra mắt
    hint: QA, SEO basics, web performance
    access: analyze
    differ_from: [dung-trang]
limits: { turns: 10, timeout: 4h, idle: 45m }
start: [blueprint]
steps:
  - id: blueprint
    type: agent
    name: Bản thiết kế tổng thể
    role: chien-luoc
    skill: website-blueprint
    prompt: |
      Make the website blueprint for this business.
      Business: {{input.business}}
      Ideal customers: {{input.customers}}
      Main action visitors should take: {{input.action}}
      Competitors named by the person: {{input.competitors}}
      Brand: {{input.brand}}

      Write it for the person, in their language: what you could not verify is marked as such, and the questions they must answer come last.
    next: duyet
    position: { x: 0, y: 0 }
  - id: duyet
    type: approve
    name: Duyệt bản thiết kế
    note: |
      Duyệt bản thiết kế tổng thể trước khi viết nội dung và dựng trang:
      {{steps.blueprint.output}}
    next: noi-dung
    position: { x: 320, y: 0 }
  - id: noi-dung
    type: agent
    name: Nội dung trang chủ
    role: noi-dung
    skill: homepage-copy
    prompt: |
      Write the homepage copy from the approved blueprint:
      {{steps.blueprint.output}}

      Proof the person gave (use nothing else as proof): {{input.proof}}
      Brand voice: {{input.brand}}
    next: dung-trang
    position: { x: 640, y: 0 }
  - id: dung-trang
    type: agent
    name: Dựng trang
    role: dung-trang
    skill: page-builder
    prompt: |
      Build the website from the approved blueprint and the copy. Use the copy as written.
      Blueprint:
      {{steps.blueprint.output}}

      Copy:
      {{steps.noi-dung.output}}

      End with the files you made and where, and what is still a placeholder.
    next: thiet-ke
    position: { x: 960, y: 0 }
  - id: thiet-ke
    type: agent
    name: Tinh chỉnh thiết kế
    role: thiet-ke
    skill: design-polish
    prompt: |
      Polish the design of the pages just built (look at them rendered, desktop and mobile), without changing the approved copy or layout.
      What was built:
      {{steps.dung-trang.output}}
    next: kiem-tra
    position: { x: 1280, y: 0 }
  - id: kiem-tra
    type: agent
    name: Kiểm tra trước khi ra mắt
    role: kiem-tra
    skill: launch-checklist
    prompt: |
      Run the pre-launch checklist on the site. Do not change anything: report each item with its evidence, launch blockers first.
      What was built:
      {{steps.dung-trang.output}}

      Design changes:
      {{steps.thiet-ke.output}}
    next: xong
    position: { x: 1600, y: 0 }
  - id: xong
    type: end
    name: Xong
    summary: |
      {{steps.dung-trang.output}}

      Kiểm tra trước khi ra mắt:
      {{steps.kiem-tra.output}}
    outputs:
      pages: "{{steps.dung-trang.output}}"
      launch_blockers: "{{steps.kiem-tra.output}}"
    position: { x: 1920, y: 0 }
---
The workflow runs by the steps in the header (steps).
