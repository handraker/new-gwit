# new-gwit

![blank](https://user-images.githubusercontent.com/58924412/147617149-62e9888f-14bc-4b34-bc95-bf288f3e60b3.png)

## desription

* 기존 gwkit의 검색이 마음에 들지 않아, 가볍게 python으로 TUI를 만들었습니다.
* urwid라던가, 더 좋은 프레임워크가 있는 것 같은데 공부하는데 시간이 많이 소요될 것 같아 간단하게 curses로 만들었습니다.
* 범용적인 실행을 위해 눈물이 나지만 python 2.7로 작성하였습니다.

## 기능

* 서버 추가
  * ctrl + n 누르면 서버를 추가할 수 있습니다.
  * 정보 입력 후, enter 를 누르면 추가되며, 취소하고 싶으면 ctrl + c 를 누릅니다.
  ![register](https://user-images.githubusercontent.com/58924412/147617148-9de1086a-ae07-4e90-ad84-fc954bd25d72.png)
* 리스트 탐색
  * 화살표 위 아래로 서버를 선택할 수 있습니다.
  * 서버 선택 후, enter 를 누르면 rlogin 으로 접속합니다.
  * 접속하는 사용자는 `/` 키를 눌러 변경할 수 있습니다.
  ![list](https://user-images.githubusercontent.com/58924412/147617147-d9933a79-4679-4e5d-ac74-b76c5d7ffb2d.png)
  * 원하는 단어로 필터링 할 수 있습니다. 스페이스바로 구분되어 and 조건으로 필터링 합니다.
  * 필터링 단어는 대소문자 구분 없이 host, tag, description 에 포함 하는 것들을 필터링합니다.
  * 필터링 된 단어는 빨간색으로 강조 됩니다.
  ![filter](https://user-images.githubusercontent.com/58924412/147617145-b82836b1-4768-4072-b691-968e608edafe.png)
* 서버 수정
  * 서버 선택 후 ctrl + e 를 누르면 정보를 수정할 수 있습니다.
  * 정보 수정 후, enter 를 누르면 추가되며, 취소하고 싶으면 ctrl + c 를 누릅니다.
  ![modify](https://user-images.githubusercontent.com/58924412/147617142-b92e6a03-7366-4c04-96c3-b10a600126d6.png)
* 기존 gwkit의 known_host 마이그레이션
  * ctrl + l 을 누르고 known_host 파일 경로를 입력합니다.
  ![migrate](https://user-images.githubusercontent.com/58924412/147617363-c3572d7e-a2b4-4fae-a443-3ac86c1fd540.png)

## 설치

* `gwkit.py` 을 그냥 실행하면 됩니다.
  * `python gwkit.py` 을 실행하거나, 실행권한을 주고 `./gwkit.py` 하시면 됩니다.
* 내부적으로 서버 목록 관리를 위해 `gwkit.py` 와 동일한 dir 에 `server_list.json` 파일을 저장합니다.
  * 아래와 같은 형식이므로 직접 수정하셔도 됩니다.
    ```json
    [
        {
            'host': '',
            'description': '',
            'tags': ['']
        }
    ]
    ```
    
## 라이센스

* 그런거 없습니다. 마음대로 가져다가 수정해서 쓰세요.....