# 상단 바와 사이드바의 페이지

앱을 영역(Pages, Drive, Finance)으로 나눠 상단 바에서 고르고, 사이드바는 지금 영역의 메뉴만 보인다. Pages 영역의 사이드바에는 Notion처럼 맨 위 페이지들을 두고, 페이지 화면은 제목과 본문만 보인다. `2026-10-02-pages-design.md`의 Pages 목록 화면(검색, Show 필터, New page 칸)과 페이지·프로젝트·할 일 화면의 하위 페이지 목록과 첨부 섹션을 이 문서가 대신한다.

## 결정

| 항목 | 결정 |
|---|---|
| 상단 바 | `monolith`(Dashboard로), 영역 탭 Pages(`/page`, `/task`), Drive(`/drive`), Finance(`/finance`), Log out. 지금 영역을 표시한다 |
| 사이드바 | 지금 영역의 메뉴만. Pages: Projects, Tasks, PAGES 제목(`/page`로)과 그 옆 `+`(새 페이지), 자기 페이지 중 맨 위에 있는 것(부모 없음, 프로젝트·할 일 페이지 아님, 휴지통 밖)을 제목 순으로, Search, Trash. Drive: Files, Trash. Finance: Assets, Expenses. Dashboard는 사이드바가 없다. 접기(«)는 그대로다 |
| 휴대폰 | 상단 바는 그대로 위에 있고, 사이드바는 아래 탭 막대가 되어 지금 영역의 메뉴를 보인다. 페이지 목록과 `+`는 숨기고 Pages 탭이 `/page`의 목록 화면을 연다 |
| `/page` | 맨 위 페이지 목록, New page 버튼, Search·Trash 링크. 따로 고르는 칸이나 필터는 없다 |
| 새 페이지 | 사이드바의 `+`와 `/page`의 New page 버튼. 제목 없이 `POST /page/pages/new`를 보내면 "Untitled" 페이지를 만들고 연다. 제목은 그 화면의 제목 칸에서 바꾼다(자동 저장) |
| 검색 | `/page/search`. 프로젝트·할 일 페이지도 찾는다(최대 100개) |
| 페이지 화면 | 경로, 제목, Delete, 본문, Linked from, Move. 하위 페이지 목록은 없다. 하위 페이지는 본문의 `[[` 링크와 사이드바(맨 위만)로 찾는다 |
| 첨부 | 페이지, 프로젝트, 할 일 화면에 첨부 섹션(Attach files, Attachments, Open in Drive)과 `…/files` 라우트가 없다. 파일은 본문에 끌어다 놓아 `…/images`로 올린다. 올린 파일은 지금처럼 그 페이지(프로젝트, 할 일)의 드라이브 폴더에 들어간다 |
| 맨 위 페이지 | 두지 않는다. 한때 `/page`를 페이지 하나로 만들었다가 되돌렸다 |

## 구현

- 영역과 메뉴는 `layout.html`이 요청 경로(`current`)로 고른다. `current`는 경로가 같거나 그 아래(`/`로 이어짐)일 때 참이다. 그래서 `/page/pages/1`이 `/page/pages/12`를 표시하지 않는다.
- Pages 영역의 사이드바가 페이지 목록을 그리도록 `page.Sidebar` 미들웨어가 요청마다 맨 위 페이지를 읽어 `web.WithSidebarPages`로 요청에 싣고, `web.Render`가 템플릿 함수 `sidebarPages`로 넘긴다. `routes.go`는 page와 task 핸들러에만 `auth.Require` 안쪽에 이 미들웨어를 씌운다.
- 휴대폰에서 사이드바의 페이지 목록과 `+`는 `.pages`로 숨기고 PAGES 제목의 링크를 탭으로 보인다.
- 스키마는 바뀌지 않는다.

## 테스트

`cmd/server/page_test.go`: 새 페이지가 `/page`와 다른 화면의 사이드바에 나온다, `/page`에는 프로젝트·할 일 페이지가 없고 검색은 찾는다, 제목 없이 만들면 Untitled, 맨 위로 옮기면 다시 목록에 나온다. 브라우저로 사이드바의 New page, 이름 바꾸기, 휴대폰 화면을 확인한다.
