# Third-party assets / 외부 자료 주의 사항

**이 파일은 권리 확인이 끝났다는 증명이 아니라, 공개·재배포 전에 점검할 목록입니다.**

- `source/assets/nasa_*`: NASA 자료로 표시된 이미지/GIF가 포함됩니다. 정확한 원본 출처, 사용조건, 출처 표시 의무와 제3자 권리(인물·로고 등)를 확인해 주세요.
- `source/assets/animal/`, `source/assets/circuit/`, `source/assets/toolbox/`: 제작자/라이선스 정보 및 외부 자료 사용 여부를 확인해 주세요.
- `source/assets/user_earth_skin_2048.webp`: 제작/제공 경로와 재배포 허용 범위를 확인해 주세요.
- 코드에 포함된 기타 텍스처·아이콘·글꼴 자료 역시 출처 확인이 필요할 수 있습니다.

루트 `LICENSE`의 MIT 문구는 **프로젝트 저작자가 사용 허가 권한을 보유한 코드와 자료**에 대해 효력이 있습니다. 타인의 저작물에 대한 권리까지 부여하지는 않습니다. 자산마다 필요하면 별도 원저작권·라이선스 고지를 추가하세요.

## WebP VP8L reader (Go Authors; BSD-3-Clause)

`source/internal/webplossless/vp8l/` is a compact, source-derived adaptation of
`golang.org/x/image/vp8l` v0.30.0, Copyright 2014 The Go Authors.
The RIFF/VP8L-only wrapper is in `source/internal/webplossless/webp.go`.
The required BSD 3-Clause license is included in
`source/internal/webplossless/LICENSE_GO_IMAGE_BSD_3_CLAUSE.txt`.
The conversion preserves original image pixels; original PNG images are retained
under `source/testdata/webp_png_reference/` and are not embedded in the EXE.
The decoder reads only the embedded VP8L lossless subset, not all WebP features.
