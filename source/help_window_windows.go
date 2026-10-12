//go:build windows

package main

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"
)

const (
	helpWindowClassName = "dotPaintHelpCenterA233"
	helpTopicListID     = 8801
	helpBodyEditID      = 8802
	helpLogButtonID     = 8803
	helpCloseButtonID   = 8804
	helpEMScrollCaret   = 0x00B7
	helpEMSetParaFormat = 0x0447
	helpPFMLinespacing  = 0x00000100
)

type helpTopic struct {
	Title string
	Body  string
}

// PARAFORMAT2 is used only for the help body's RichEdit line spacing.
// The A233 help centre uses an 11 pt font (two points above the former
// default 9 pt UI font) and an exact line box two pixels taller.
type helpParaFormat2 struct {
	CbSize           uint32
	DwMask           uint32
	WNumbering       uint16
	WReserved        uint16
	DxStartIndent    int32
	DxRightIndent    int32
	DxOffset         int32
	WAlignment       uint16
	CTabCount        int16
	RgxTabs          [32]int32
	DySpaceBefore    int32
	DySpaceAfter     int32
	DyLineSpacing    int32
	SStyle           int16
	BLineSpacingRule byte
	BOutlineLevel    byte
	WShadingWeight   uint16
	WShadingStyle    uint16
	WNumberingStart  uint16
	WNumberingStyle  uint16
	WNumberingTab    uint16
	WBorderSpace     uint16
	WBorderWidth     uint16
	WBorders         uint16
}

type helpWindowState struct {
	owner      *App
	hwnd       uintptr
	topicList  uintptr
	bodyEdit   uintptr
	versionLbl uintptr
	logButton  uintptr
	closeBtn   uintptr
	topicFont  uintptr
	bodyFont   uintptr
	bodyRich   bool
	topics     []helpTopic
	selected   int
}

var (
	helpWindowWndProcCallback uintptr
	helpWindowClassRegistered bool
	activeHelpWindow          *helpWindowState
)

func dotPaintHelpTopics() []helpTopic {
	gpuVerification := "GPU 기능은 준비되지 않을 경우 CPU 경로로 자동 전환됩니다."
	if diagnosticLoggingEnabled {
		gpuVerification = "정확한 사용 여부는 dotPaint.log의 backend=D3D11, fallback=false 문구로 확인할 수 있습니다."
	}
	topics := []helpTopic{
		{
			Title: "1. 시작하기와 화면 구성",
			Body: `dotPaint는 도트 작업과 물리·수학 설명 그림을 빠르고 정확하게 만드는 Windows 그림 편집기입니다.

기본 화면
• 왼쪽: 도구상자와 선택한 도구의 세부 옵션
• 가운데: 실제 캔버스
• 아래: 전경색·배경색 색상표와 상태표시줄
• 오른쪽: 미리보기, 클립보드 기록, 히스토리 창

기본 사용 순서
1. 왼쪽에서 도구를 고릅니다.
2. 필요하면 굵기·각도 제한·안티에일리어싱 같은 옵션을 정합니다.
3. 캔버스에서 클릭하거나 끌어 작업합니다.
4. 상태표시줄에서 좌표·크기·길이·각도를 확인합니다.
5. Ctrl+S로 저장합니다.

오른쪽 미리보기는 4배 이상 확대했을 때 사용할 수 있습니다. 미리보기에서 원하는 위치를 클릭하면 그 지점이 작업 화면 중앙으로 이동합니다.`,
		},
		{
			Title: "2. 파일 열기·저장·최근 파일",
			Body: `파일 메뉴에서 새 그림, 열기, 저장, 다른 이름으로 저장, 최근 파일을 사용할 수 있습니다.

지원 형식
• 읽기: PNG, BMP, JPG/JPEG, GIF
• 저장: PNG, BMP, JPG/JPEG

투명 픽셀이 있는 그림은 PNG로 저장하는 것이 안전합니다. JPEG는 투명도를 저장하지 못합니다.

다른 프로그램이나 Windows 탐색기에서 이미지 파일을 복사한 뒤 dotPaint에서 Ctrl+V하면 캔버스 왼쪽 위에 떠 있는 선택 영역으로 붙습니다.

파일을 dotPaint 창에 끌어다 놓아 열 수도 있습니다. 저장하지 않은 변경 사항이 있으면 새 파일을 열거나 종료할 때 저장 여부를 묻습니다.`,
		},
		{
			Title: "3. 캔버스·확대·눈금·미리보기",
			Body: `확대/축소는 그림 픽셀만 확대하며 메뉴·도구상자·색상표는 고정됩니다.

주요 조작
• Ctrl+Page Up / Ctrl+Page Down: 단계별 확대·축소
• 확대/축소 도구: 원하는 영역을 골라 확대
• 보기 > 보통 크기 / 큰 크기: 정해진 배율로 변경
• 6배 이상: 픽셀 눈금 표시 가능
• 4배 이상: 오른쪽 미리보기 사용 가능

돋보기로 영역을 확대한 뒤에는 직전에 사용하던 도구로 자동 복귀합니다.

캔버스 크기는 이미지 > 캔버스 크기에서 숫자로 입력하거나, 캔버스 주변 8개 조절점을 끌어 자르거나 확장할 수 있습니다. 확장된 부분은 현재 배경색으로 채워집니다.`,
		},
		{
			Title: "4. 선택 도구",
			Body: `선택 도구는 자유 선택, 사각 선택, 색 영역 선택, 동일 색 전체 선택의 네 종류입니다.

사각 선택
• 캔버스 안에서 끌어 사각형을 만듭니다.
• 선택 바깥을 클릭하면 선택이 취소됩니다.
• 선택 바깥에서 5px 이상 끌면 기존 선택을 취소하고 새 선택을 만듭니다.
• 캔버스 바깥 회색 영역에서 안쪽으로 끌면 가장 가까운 최외각 픽셀부터 선택됩니다.
• 상태표시줄에 선택 가로×세로 크기가 표시됩니다.

자유 선택
• 손으로 그린 윤곽 내부만 선택합니다.

색 선택
• 색 영역 선택: 클릭한 지점과 이어진 유사 색만 선택
• 동일 색 전체 선택: 캔버스 전체의 유사 색을 모두 선택
• 허용 불일치 0~100으로 색 차이 범위를 정합니다.

공통
• Shift를 누른 채 새 선택: 기존 선택에 추가
• 선택 내부 드래그: 이동
• 이동 중 Shift: 수평·수직·45° 방향 제한
• Ctrl+A: 전체 선택
• Del: 배경색으로 삭제
• Shift+Del: 완전 투명으로 삭제`,
		},
		{
			Title: "5. 복사·붙여넣기·투명도",
			Body: `Ctrl+C, Ctrl+X, Ctrl+V로 선택 영역을 복사·잘라내기·붙여넣기할 수 있습니다.

투명 클립보드
• dotPaint는 복사할 때 PNG 알파 정보와 구형 프로그램용 DIB를 함께 기록합니다.
• dotPaint끼리 복사·붙여넣기하거나 PNG를 지원하는 프로그램과 주고받을 때 투명도가 유지됩니다.
• 붙여넣은 이미지는 떠 있는 선택 영역이 되어 위치를 바꿀 수 있습니다.

선택 옵션의 불투명/투명
• 불투명: 선택 이미지의 투명 부분을 배경색으로 처리
• 투명: 알파가 0인 픽셀과 지정한 투명색이 아래 그림을 비치게 함

오른쪽 클립보드 창에는 최근 이미지 6개가 기록됩니다. 핀을 누르면 새 이미지가 들어와도 유지됩니다. Print Screen과 다른 프로그램에서 복사한 이미지도 자동 기록됩니다.`,
		},
		{
			Title: "6. 붓·연필·지우개·색 채우기",
			Body: `통합 붓 도구에서 연필형, 붓형, 에어브러시형 등 여러 그리기 모양을 선택할 수 있습니다.

굵기
• 붓과 지우개는 1~40px 범위
• 커서 주변의 미리보기 테두리로 실제 적용 범위를 확인

지우개
• 배경색으로 지우거나 투명 처리 옵션을 사용할 수 있습니다.

색 채우기
• 클릭한 지점과 이어진 같은 계열의 색을 채웁니다.
• 허용 불일치를 높이면 비슷한 색까지 함께 채웁니다.

색 골라내기(스포이드)
• 캔버스뿐 아니라 화면 바깥의 다른 프로그램 색도 가져올 수 있습니다.
• 색을 찍은 뒤에는 이전에 사용하던 도구로 자동 복귀합니다.

대부분의 사용자 커서는 현재 픽셀 명도 128을 기준으로 밝고 어두운 형태가 자동 전환됩니다. 스포이드 커서는 원본 모양을 그대로 사용합니다.`,
		},
		{
			Title: "7. 직선·화살표·직각삼각형",
			Body: `세 도구는 같은 정밀 선 엔진을 사용합니다.

각도 제한
• 15×n°: 0°, 15°, 30°… 단위로 제한
• tan n: 1/5, 1/4, 1/3, 1/2, 2/3, 1, 3/2, 2, 3, 4, 5의 정수 도트 계단
• 제한은 Shift를 누르는 동안 적용됩니다.

직선
• 굵기 1~40px
• 안티에일리어싱 켬/끔
• 드래그 중 최종 픽셀과 같은 미리보기 표시

화살표
• 몸통 굵기 1~8px
• 화살촉 모양 선택 가능
• 직선과 같은 각도·tan n 제한 사용

직각삼각형
• 드래그한 선이 빗변이 됩니다.
• 첫 점에서 가로선, 둘째 점에서 세로선이 만나 직각을 만듭니다.
• 직선과 같은 제한·굵기·안티에일리어싱을 사용합니다.

상태표시줄에는 길이, 가로·세로 변화량, 각도 또는 tan 값이 표시됩니다.`,
		},
		{
			Title: "8. 곡선과 기본 도형",
			Body: `곡선
1. 먼저 기준선을 긋습니다.
2. 왼쪽 옵션에서 고른 1~4개의 굽힘점을 차례로 끕니다.
3. 마지막 굽힘점을 놓으면 확정됩니다.

사각형
• 마우스 위치는 선의 중심이 아니라 바깥 테두리입니다.
• Shift를 누르면 정사각형이 됩니다.
• 굵은 선도 표시된 바깥 크기 안쪽으로 자랍니다.

타원·원
• Shift를 누르면 원으로 제한됩니다.
• 대칭과 꼭짓점 기준 옵션을 사용할 수 있습니다.

다각형·둥근 사각형 등도 전경색을 선 색, 배경색을 채움 색으로 사용합니다. 도형 옵션에서 외곽선만, 채움만, 외곽선+채움을 선택할 수 있습니다.`,
		},
		{
			Title: "9. 텍스트 도구",
			Body: `1. 텍스트 도구를 고릅니다.
2. 캔버스에서 끌어 텍스트 상자를 만듭니다.
3. 글꼴, 크기, 한글·숫자·기호·영문 표시, 굵게(B), 기울임(I), 밑줄(U)을 조정합니다.
4. 상자 밖을 클릭하거나 다른 도구를 고르면 그림에 확정됩니다.

편집 단축키
• Ctrl+A: 텍스트 전체 선택
• Ctrl+C / Ctrl+X / Ctrl+V: 텍스트 복사·잘라내기·붙여넣기
• 방향키: 커서 이동
• Shift+방향키: 범위 선택

선택한 글자가 있으면 서식은 그 범위에 적용됩니다. 선택이 없으면 다음 입력 글자의 서식으로 사용됩니다.

한글 입력은 Windows IME 조합을 추적하며, 조합 중 바깥을 클릭하면 먼저 글자를 확정한 뒤 텍스트 상자를 종료합니다. 마지막 글자 서식과 다음 입력 서식은 서로 분리되어 있습니다.`,
		},
		{
			Title: "10. 색상표·전경색·배경색",
			Body: `색상표에서
• 왼쪽 클릭: 전경색
• 오른쪽 클릭: 배경색

전경색은 붓·직선·글자·도형 외곽선에 사용됩니다. 배경색은 지우개, Del 삭제, 도형 채움에 사용됩니다.

색상표를 더블클릭하면 사용자 색을 편집할 수 있습니다. 투명색은 별도의 알파 0 픽셀로 관리되므로 검정색과 다릅니다.

다크모드는 캔버스 픽셀과 전경색·배경색을 바꾸지 않습니다. 다만 수식·그래프·다차원 도형 편집기의 ‘검은 배경’은 미리보기 바탕만 검게 바꿉니다. 수식 글자, 그래프와 축, 도형의 선·면 색은 현재 전경색·배경색을 그대로 유지합니다. 다크모드에서는 자동으로 켜지고 라이트모드에서는 자동으로 꺼집니다.`,
		},
		{
			Title: "11. 실행 취소·히스토리",
			Body: `붓, 지우개, 선, 도형, 텍스트, 붙여넣기, 선택 이동처럼 캔버스에 확정된 작업은 히스토리에 기록됩니다.

단축키
• Ctrl+Z: 실행 취소
• Ctrl+Shift+Z: 다시 실행
• Ctrl+Y: 다시 실행

보기 > 히스토리 창을 켜면 작업 목록을 직접 클릭해 원하는 시점으로 이동할 수 있습니다.

과거 시점으로 돌아간 뒤 새 작업을 하면 그 이후의 미래 기록은 제거됩니다. 캔버스 크기 변경도 독립된 기록으로 저장됩니다.`,
		},
		{
			Title: "12. 이미지 변형",
			Body: `늘이기/기울이기
• 이미지 > 늘이기/기울이기 또는 Ctrl+W
• 가로·세로 비율 1~500%
• 가로·세로 기울이기 -89.0~89.0°
• 안티에일리어싱 끔: 도트 작업용 최근접 픽셀
• 안티에일리어싱 켬: 알파를 고려한 부드러운 보간

대칭 이동 및 회전
• 좌우·상하 대칭
• 90°, 180°, 270°와 소수점 첫째 자리 임의 각도 회전
• 선택 영역이 있으면 선택에만 적용

선택이나 활성 텍스트가 없으면 전체 캔버스에 적용됩니다. 변형 결과의 빈 부분은 투명 픽셀로 처리됩니다.`,
		},
		{
			Title: "13. 수식 편집기",
			Body: `삽입 > 수식에서 독립된 수식 편집기를 엽니다.

주요 기능
• 위첨자·아래첨자
• 분수
• 극한, 합, 적분
• 벡터 화살표
• 행렬·집합·괄호
• sin, log, π, e, α와 여러 수학 기호
• 굵게·기울임·크기 조절
• 검은 배경: 미리보기 바탕만 검게 표시하며 수식 색은 현재 전경색 유지

입력 규칙
• Space: 현재 수식 블록을 끝내고 다음 위치로 이동
• 역따옴표 문자: 수식 안에 실제 공백 입력

미리보기 복사는 투명 PNG로 클립보드에 복사합니다. 스크립트 복사는 다시 편집할 수 있는 수식 명령 문자열을 복사합니다. 삽입을 누르면 현재 수식이 투명한 떠 있는 선택 이미지로 캔버스에 들어갑니다. 다크모드에서 창을 열면 검은 배경이 기본으로 켜지며, 버튼으로 개별 변경할 수 있습니다.`,
		},
		{
			Title: "14. 함수·그래프 편집기",
			Body: `삽입 > 함수/그래프에서 그래프 편집기를 엽니다.

지원 형태
• y=f(x)
• x=f(y)
• 극좌표 r=f(θ)
• 차수 제한 없는 다항식
• sin, cos, tan, csc, sec, cot
• 역삼각함수
• 지수·로그 함수

설정
• x·y 범위
• 라디안/도 단위
• 좌표축·눈금·화살표
• 정비례 4:3 옵션
• 극좌표 표본 수
• 검은 배경: 미리보기 바탕만 검게 표시하며 그래프·축 색은 현재 전경색 유지

그래프는 640×480 투명 이미지로 만들어집니다. 미리보기 복사, 스크립트 복사, 캔버스 삽입을 지원합니다. 다크모드에서 창을 열면 검은 배경이 기본으로 켜집니다.`,
		},
		{
			Title: "15. 회로·동물·사용자 이미지",
			Body: `삽입 > 이미지에서 준비된 이미지를 선택해 캔버스에 넣을 수 있습니다.

전기회로
• 스위치, 다이오드, 저항, 가변저항, 축전기
• 전류계, 전압계, 코일, 교류·직류 전원
• NPN·PNP 트랜지스터

동물
• 아델리 펭귄1
• 아델리 펭귄2
• 쥐

사용자 정의 이미지
• 클립보드의 300×300px 이하 이미지를 저장
• 최대 20개 슬롯
• PNG 투명도 유지
• 프로그램을 종료해도 다음 실행에 유지
• 실행 파일 옆 user_images 폴더 또는 사용자 설정 폴더에 저장

모든 항목은 클릭으로 선택하고, 더블클릭·Enter·삽입 버튼으로 떠 있는 선택 이미지로 배치합니다.`,
		},
		{
			Title: "16. 다차원 도형 편집기",
			Body: `삽입 > 이미지 > 다차원 도형에서 3차원·4차원 도형을 회전하고 현재 모습을 캔버스에 넣습니다.

3차원 예
• 구·타원체, 원기둥, 원뿔, 다각뿔
• 정사면체·정육면체·정팔면체
• 풀러렌 C60
• 2·3-스타인메츠 입체
• 토러스, 뫼비우스 띠

4차원 예
• 테서렉트
• 정5포체, 정24포체, 정120포체, 정600포체
• 클라인 병

표시 방식
• 불투명: 면 깊이 버퍼 사용
• 반투명: 면과 선의 가림을 계산해 표시
• 투명: 앞쪽 면 개수에 따라 선을 감쇠
• 앞면 실선, 뒷면 점선
• 부드럽게, 검은 배경(바탕만 변경), 축 표시
• 다크모드에서는 검은 배경이 자동으로 켜지고 라이트모드에서는 자동으로 꺼짐

마우스로 미리보기를 끌어 회전하고, 축 보이기를 켠 뒤 축을 따로 옮길 수 있습니다. ‘도형 제자리’와 ‘축 제자리’로 각각 초기화합니다.`,
		},
		{
			Title: "17. 천체·블랙홀·GPU 가속",
			Body: `다차원 도형 편집기에는 지구, 달, 토성과 블랙홀 표시가 포함되어 있습니다.

천체
• 지구는 실행 파일 옆 skins\\earth.png로 교체 가능
• 권장 지도는 2:1 비율의 정방형 세계지도
• 외부 PNG가 없으면 내장 스킨 사용

블랙홀
• 실제 블랙홀·강착원반 보기
• 실제 모습+측지선
• 측지선 도해
• 3차원 곡률 좌표망
• 좌표망 밀도 3~17줄/축
• Kerr 회전량 a*=0.00~0.99는 곡률 좌표망에서만 사용

GPU 가속
• D3D11 계산 셰이더로 블랙홀, 천체, 불투명·반투명 면, 선 가림을 계산
• GPU가 준비되지 않으면 CPU 경로로 자동 대체
• 직선·화살표·직각삼각형 미리보기는 GPU 왕복보다 빠른 CPU 지역 렌더를 사용

` + gpuVerification,
		},
		{
			Title: "18. 다크모드와 표시 설정",
			Body: `보기 > 다크모드는 프로그램 UI와 편집기 미리보기의 대비를 함께 조정합니다.

그대로 유지되는 항목
• 캔버스의 기존 픽셀
• 전경색·배경색
• 도구 설정과 클릭 영역
• 메뉴 접근키와 단축키

자동으로 바뀌는 항목
• 다크모드: 수식·그래프·다차원 도형의 검은 배경 켬
• 라이트모드: 세 편집기의 검은 배경 끔

검은 배경은 해당 편집기 안의 버튼이나 확인란으로 다시 바꿀 수 있습니다. 다크모드에서는 메뉴, 패널, 제목 표시줄, 버튼, 입력 상자와 스크롤바를 어둡게 표시하며 라이트모드와 배치·기능은 같습니다.

비트맵 보기
• 메뉴·도구상자·색상표 등 주변 UI를 숨깁니다.
• 캔버스 편집은 계속 가능합니다.

도구상자, 색상표, 상태표시줄, 클립보드, 히스토리, 눈금, 미리보기는 보기 메뉴에서 개별적으로 켜고 끌 수 있습니다.`,
		},
		{
			Title: "19. 주요 단축키",
			Body: `파일
• Ctrl+N: 새 그림
• Ctrl+O: 열기
• Ctrl+S: 저장

편집
• Ctrl+Z: 실행 취소
• Ctrl+Shift+Z 또는 Ctrl+Y: 다시 실행
• Ctrl+A: 전체 선택
• Ctrl+C / Ctrl+X / Ctrl+V: 복사 / 잘라내기 / 붙여넣기
• Del: 배경색으로 삭제
• Shift+Del: 투명으로 삭제

보기·변형
• Ctrl+Page Up / Ctrl+Page Down: 확대 / 축소
• Ctrl+W: 늘이기/기울이기

삽입
• Ctrl+N을 누른 상태에서 M: 수식 편집기
• Ctrl+N을 누른 상태에서 G: 함수/그래프 편집기

도움말
• F1: 도움말 센터
• Esc: 진행 중인 임시 작업 또는 보조 창 닫기

메뉴의 밑줄 문자는 Alt와 함께 눌러 접근할 수 있습니다.`,
		},
	}
	if diagnosticLoggingEnabled {
		topics = append(topics, helpTopic{
			Title: "20. 문제 해결·진단 로그",
			Body: `문제가 생기면 도움말 > 진단 로그 열기를 선택합니다. 실행 파일 옆의 dotPaint.log가 열립니다.

로그에 기록되는 내용
• 프로그램 버전과 Windows 아키텍처
• 파일 열기·저장
• 클립보드 형식과 투명도
• 텍스트 IME·서식 처리
• 선택·도형·직선 미리보기
• 다차원 도형 GPU 초기화와 렌더 경로
• 종료·오류·예외

오류를 보고할 때 함께 보내면 좋은 것
1. 사용한 EXE 버전
2. 문제가 보이는 화면 캡처
3. 재현 순서
4. dotPaint.log

GPU 기능은 로그의 backend=D3D11 및 fallback=false를 확인하십시오. 프로그램이 비정상 종료되지 않았더라도 마지막 로그 줄이 문제 위치를 찾는 데 중요합니다.`,
		})
	}
	versionNumber := 20
	if diagnosticLoggingEnabled {
		versionNumber = 21
	}
	topics = append(topics, helpTopic{
		Title: fmt.Sprintf("%d. 버전 정보와 정식 1.3", versionNumber),
		Body: fmt.Sprintf(`현재 실행 버전: %s

이 버전은 픽셀 편집, 도형과 수식, 그래프, 다차원 도형, 물리장 그리기를 통합한 dotPaint 1.3 정식판입니다.

핵심 목표
• 픽셀 단위로 정확한 도트 작업
• 물리·수학 설명과 시험 문제용 정비례 도해
• 빠른 직선·화살표·도형·텍스트 배치
• 투명색과 클립보드의 안정적인 왕복
• 수식·그래프·회로·다차원 도형의 통합 삽입
• 라이트·다크모드의 동일한 기능과 배치

dotPaint 1.3은 Windows x64 및 x86 실행 파일을 제공합니다. 운영체제에 맞는 실행 파일을 사용하십시오.`, appVersion),
	})
	return topics
}

func registerHelpWindowClass(hinst uintptr) bool {
	if helpWindowClassRegistered {
		return true
	}
	if helpWindowWndProcCallback == 0 {
		helpWindowWndProcCallback = syscall.NewCallback(helpWindowWndProc)
	}
	cursor, _, _ := procLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEXW{
		CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
		Style:         CS_HREDRAW | CS_VREDRAW,
		LpfnWndProc:   helpWindowWndProcCallback,
		HInstance:     hinst,
		HCursor:       cursor,
		HbrBackground: uintptr(COLOR_BTNFACE + 1),
		LpszClassName: utf16Ptr(helpWindowClassName),
	}
	atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		logf("HELP_CENTER register class failed: %v", err)
		return false
	}
	helpWindowClassRegistered = true
	return true
}

func helpReadableFont(hwnd uintptr, pointSize int) uintptr {
	dpi := editorDPIValue(hwnd, 96)
	pixels := max(1, (pointSize*dpi+36)/72)
	return createGDIFont("Segoe UI", pixels, false, false, false)
}

func helpApplyReadableBodyFormat(s *helpWindowState) {
	if s == nil || s.bodyEdit == 0 {
		return
	}
	if s.bodyFont != 0 {
		sendMessage(s.bodyEdit, WM_SETFONT, s.bodyFont, 1)
	}
	if !s.bodyRich {
		return
	}
	textColor := rgb(20, 20, 20)
	backgroundColor := rgb(255, 255, 255)
	if s.owner != nil && s.owner.darkMode {
		textColor = rgb(238, 238, 238)
		backgroundColor = rgb(18, 18, 18)
	}
	cf := CHARFORMATW{
		CbSize:      uint32(unsafe.Sizeof(CHARFORMATW{})),
		DwMask:      CFM_SIZE | CFM_FACE | CFM_COLOR,
		YHeight:     11 * 20,
		CrTextColor: textColor,
	}
	copy(cf.SzFaceName[:], syscall.StringToUTF16("Segoe UI"))
	sendMessage(s.bodyEdit, EM_SETSEL, 0, ^uintptr(0))
	sendMessage(s.bodyEdit, EM_SETCHARFORMAT, SCF_ALL, uintptr(unsafe.Pointer(&cf)))
	sendMessage(s.bodyEdit, EM_SETBKGNDCOLOR, 0, uintptr(backgroundColor))

	// RichEdit rule 4 means "at least" the supplied height in twips.
	// A232 accidentally used rule 5, where DyLineSpacing is interpreted as
	// twentieths of a line; passing a twip value therefore created roughly
	// twelve lines of vertical space.  Rule 4 gives the intended 11 pt line
	// box plus exactly two device pixels.
	dpi := editorDPIValue(s.hwnd, 96)
	linePixels := max(1, (11*dpi+36)/72+2)
	pf := helpParaFormat2{
		CbSize:           uint32(unsafe.Sizeof(helpParaFormat2{})),
		DwMask:           helpPFMLinespacing,
		DyLineSpacing:    int32((linePixels*1440 + dpi/2) / dpi),
		BLineSpacingRule: 4,
	}
	sendMessage(s.bodyEdit, helpEMSetParaFormat, 0, uintptr(unsafe.Pointer(&pf)))
	sendMessage(s.bodyEdit, EM_SETSEL, 0, 0)
}

func helpCreateChild(s *helpWindowState, class, text string, style uintptr, id int) uintptr {
	style = ownerDrawPushButtonStyle(class, style)
	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(class))),
		uintptr(unsafe.Pointer(utf16Ptr(text))),
		WS_CHILD|WS_VISIBLE|style,
		0, 0, 100, 30,
		s.hwnd, uintptr(id), s.owner.hinst, 0,
	)
	if hwnd == 0 {
		logf("HELP_CENTER child create failed class=%q id=%d err=%v", class, id, err)
		return 0
	}
	font, _, _ := procGetStockObject.Call(DEFAULT_GUI_FONT)
	sendMessage(hwnd, WM_SETFONT, font, 1)
	applyDarkThemeToWindow(hwnd, s.owner.darkMode)
	return hwnd
}

func (a *App) showHelpCenter() {
	if activeHelpWindow != nil && activeHelpWindow.hwnd != 0 {
		if ok, _, _ := procIsWindow.Call(activeHelpWindow.hwnd); ok != 0 {
			procShowWindow.Call(activeHelpWindow.hwnd, SW_SHOW)
			procSetForegroundWindow.Call(activeHelpWindow.hwnd)
			return
		}
	}
	if !registerHelpWindowClass(a.hinst) {
		messageBox(a.hwnd, "도움말 창을 만들 수 없습니다.", "dotPaint 도움말", MB_OK|MB_ICONERROR)
		return
	}
	state := &helpWindowState{owner: a, topics: dotPaintHelpTopics(), selected: 0}
	activeHelpWindow = state
	hwnd, _, err := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(utf16Ptr(helpWindowClassName))),
		uintptr(unsafe.Pointer(utf16Ptr("dotPaint 도움말 센터"))),
		WS_OVERLAPPEDWINDOW,
		uintptr(CW_USEDEFAULT), uintptr(CW_USEDEFAULT), 1040, 740,
		a.hwnd, 0, a.hinst, 0,
	)
	if hwnd == 0 {
		activeHelpWindow = nil
		logf("HELP_CENTER create failed: %v", err)
		messageBox(a.hwnd, "도움말 창을 만들 수 없습니다.", "dotPaint 도움말", MB_OK|MB_ICONERROR)
		return
	}
	state.hwnd = hwnd
	showThemedEditorWindow(hwnd, a.darkMode)
	a.statusText = "도움말 센터를 열었습니다. 왼쪽 항목을 선택하십시오."
	logf("HELP_CENTER opened topics=%d version=%s", len(state.topics), appVersion)
	invalidate(a.hwnd)
}

func helpLayout(s *helpWindowState) {
	if s == nil || s.hwnd == 0 {
		return
	}
	var rc RECT
	procGetClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&rc)))
	w := int(rc.Right - rc.Left)
	h := int(rc.Bottom - rc.Top)
	margin := 12
	gap := 10
	footerH := 46
	listW := clamp(w/4, 220, 300)
	contentTop := margin
	contentBottom := max(contentTop+100, h-margin-footerH)
	if s.topicList != 0 {
		procMoveWindow.Call(s.topicList, uintptr(margin), uintptr(contentTop), uintptr(max(100, listW)), uintptr(max(80, contentBottom-contentTop)), 1)
	}
	bodyX := margin + listW + gap
	if s.bodyEdit != 0 {
		procMoveWindow.Call(s.bodyEdit, uintptr(bodyX), uintptr(contentTop), uintptr(max(100, w-bodyX-margin)), uintptr(max(80, contentBottom-contentTop)), 1)
	}
	footerY := max(margin, h-footerH+6)
	if s.versionLbl != 0 {
		procMoveWindow.Call(s.versionLbl, uintptr(margin), uintptr(footerY), uintptr(max(120, w-340)), 30, 1)
	}
	if s.logButton != 0 {
		procMoveWindow.Call(s.logButton, uintptr(max(margin, w-322)), uintptr(footerY), 150, 30, 1)
	}
	if s.closeBtn != 0 {
		procMoveWindow.Call(s.closeBtn, uintptr(max(margin, w-162)), uintptr(footerY), 150, 30, 1)
	}
}

func helpSetSelectedTopic(s *helpWindowState, index int) {
	if s == nil || len(s.topics) == 0 {
		return
	}
	index = clamp(index, 0, len(s.topics)-1)
	s.selected = index
	if s.topicList != 0 {
		procSendMessageW.Call(s.topicList, LB_SETCURSEL, uintptr(index), 0)
	}
	if s.bodyEdit != 0 {
		body := strings.ReplaceAll(s.topics[index].Body, "\n", "\r\n")
		procSetWindowTextW.Call(s.bodyEdit, uintptr(unsafe.Pointer(utf16Ptr(body))))
		helpApplyReadableBodyFormat(s)
		procSendMessageW.Call(s.bodyEdit, EM_SETSEL, 0, 0)
		procSendMessageW.Call(s.bodyEdit, helpEMScrollCaret, 0, 0)
	}
	procSetWindowTextW.Call(s.hwnd, uintptr(unsafe.Pointer(utf16Ptr("dotPaint 도움말 — "+s.topics[index].Title))))
	logf("HELP_CENTER select index=%d title=%q", index, s.topics[index].Title)
}

func helpRefreshTheme(s *helpWindowState) {
	if s == nil || s.hwnd == 0 || s.owner == nil {
		return
	}
	themeWindowHandles(s.owner.darkMode, s.hwnd, s.topicList, s.bodyEdit, s.versionLbl, s.logButton, s.closeBtn)
	helpApplyReadableBodyFormat(s)
	drawDarkEditorCaption(s.hwnd)
	procInvalidateRect.Call(s.hwnd, 0, 1)
}

func helpWindowWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	defer func() {
		if r := recover(); r != nil {
			logPanic("helpWindowWndProc", r)
		}
	}()
	if r, handled := handleDarkEditorNonClient(hwnd, msg, wparam, lparam); handled {
		return r
	}
	s := activeHelpWindow
	switch msg {
	case WM_CREATE:
		if s == nil {
			return ^uintptr(0)
		}
		s.hwnd = hwnd
		s.topicList = helpCreateChild(s, "LISTBOX", "", WS_BORDER|WS_TABSTOP|WS_VSCROLL|LBS_NOTIFY|LBS_HASSTRINGS, helpTopicListID)
		bodyClass := "EDIT"
		if err := modMsftedit.Load(); err == nil {
			bodyClass = "RICHEDIT50W"
			s.bodyRich = true
		} else {
			logf("HELP_CENTER RichEdit load failed: %v", err)
		}
		s.bodyEdit = helpCreateChild(s, bodyClass, "", WS_BORDER|WS_TABSTOP|WS_VSCROLL|ES_MULTILINE|ES_AUTOVSCROLL|esReadOnly, helpBodyEditID)
		s.topicFont = helpReadableFont(hwnd, 11)
		s.bodyFont = helpReadableFont(hwnd, 11)
		if s.topicFont != 0 {
			sendMessage(s.topicList, WM_SETFONT, s.topicFont, 1)
		}
		helpApplyReadableBodyFormat(s)
		s.versionLbl = helpCreateChild(s, "STATIC", fmt.Sprintf("현재 버전: %s · F1로 다시 열기", appVersion), 0, 0)
		if diagnosticLoggingEnabled {
			s.logButton = helpCreateChild(s, "BUTTON", "진단 로그 열기", WS_TABSTOP, helpLogButtonID)
		}
		s.closeBtn = helpCreateChild(s, "BUTTON", "닫기", WS_TABSTOP, helpCloseButtonID)
		for _, topic := range s.topics {
			procSendMessageW.Call(s.topicList, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(topic.Title))))
		}
		helpLayout(s)
		helpSetSelectedTopic(s, 0)
		helpRefreshTheme(s)
		procSetFocus.Call(s.topicList)
		return 0
	case WM_SIZE:
		if s != nil && s.hwnd == hwnd {
			helpLayout(s)
		}
		return 0
	case WM_COMMAND:
		if s != nil && s.hwnd == hwnd {
			id := int(wparam & 0xffff)
			code := int((wparam >> 16) & 0xffff)
			switch {
			case id == helpTopicListID && code == LBN_SELCHANGE:
				idx, _, _ := procSendMessageW.Call(s.topicList, LB_GETCURSEL, 0, 0)
				if int(idx) >= 0 && int(idx) < len(s.topics) {
					helpSetSelectedTopic(s, int(idx))
				}
			case diagnosticLoggingEnabled && id == helpLogButtonID:
				openDiagnosticLog(hwnd)
			case id == helpCloseButtonID:
				procDestroyWindow.Call(hwnd)
			}
		}
		return 0
	case WM_DRAWITEM:
		if drawThemedOwnerDrawPushButton((*DRAWITEMSTRUCT)(unsafe.Pointer(lparam))) {
			return 1
		}
	case WM_KEYDOWN:
		if wparam == VK_ESCAPE {
			procDestroyWindow.Call(hwnd)
			return 0
		}
	case WM_CTLCOLOREDIT, WM_CTLCOLORLISTBOX:
		if s != nil && s.owner != nil && s.owner.darkMode && wparam != 0 {
			procSetBkMode.Call(wparam, OPAQUE)
			procSetTextColor.Call(wparam, uintptr(rgb(235, 235, 235)))
			procSetBkColor.Call(wparam, uintptr(rgb(30, 30, 30)))
			if s.owner.darkBrush == 0 {
				s.owner.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
			}
			return s.owner.darkBrush
		}
	case WM_CTLCOLORSTATIC, WM_CTLCOLORBTN, WM_CTLCOLORDLG:
		if s != nil && s.owner != nil && s.owner.darkMode && wparam != 0 {
			procSetBkMode.Call(wparam, TRANSPARENT)
			procSetTextColor.Call(wparam, uintptr(rgb(235, 235, 235)))
			if s.owner.darkBrush == 0 {
				s.owner.darkBrush, _, _ = procCreateSolidBrush.Call(uintptr(rgb(30, 30, 30)))
			}
			return s.owner.darkBrush
		}
	case WM_PAINT:
		if s != nil && s.hwnd == hwnd {
			var ps PAINTSTRUCT
			hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			var rc RECT
			procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc)))
			bg := rgb(240, 240, 240)
			if s.owner != nil && s.owner.darkMode {
				bg = rgb(30, 30, 30)
			}
			fillRect(hdc, rc, bg)
			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
			return 0
		}
	case WM_ERASEBKGND:
		return 1
	case WM_CLOSE:
		procDestroyWindow.Call(hwnd)
		return 0
	case WM_DESTROY:
		if s != nil && s.hwnd == hwnd {
			if s.topicFont != 0 {
				procDeleteObject.Call(s.topicFont)
				s.topicFont = 0
			}
			if s.bodyFont != 0 {
				procDeleteObject.Call(s.bodyFont)
				s.bodyFont = 0
			}
			logf("HELP_CENTER closed selected=%d", s.selected)
			activeHelpWindow = nil
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}
