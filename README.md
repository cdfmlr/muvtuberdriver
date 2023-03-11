# muvtuber driver

这个程序驱动整个 muvtuber 的运行，即协调各个模块，实现数据流动。

## 启动

THE FOLLOWING DOCS ARE OUTDATED!
GOTO https://github.com/cdfmlr/muvtuber/blob/main/README.md FOR A UPDATED VERSION.

### chatbot_api

```sh
cd chatbot_api
poetry shell
gunicorn start:app -c ./gunicorn.conf.py
```

Chatbot Api 运行在 `:8080`:

```sh
curl 'http://localhost:8080/chatbot/get_response?chat=文本内容'
```

### emotext

```sh
cd emotext
pyenv local 3.8.16
poetry shell
PYTHONPATH=$PYTHONPATH:. python emotext/httpapi.py --port 9003
```

Emotext 运行在 `:9003`:

```sh
curl -X POST 'http://localhost:9003/' -d '文本内容'
```

### live2ddriver

```sh
cd live2ddriver
go run . -shizuku localhost:9004 -verbose
```

Live2DDriver 运行在 `:9004`:

```sh
curl -X POST 'http://localhost:9004/' -d '文本内容'
```

### blivedm

```sh
cd blivedm
poetry shell
python main.py
```

Blivedm 运行在 `:12450`:

```js
let chat = new WebSocket('ws://localhost:12450/api/chat');
chat.onmessage = (e) => {console.log(JSON.parse(e.data))};
chat.send(JSON.stringify({"cmd":1,"data":{"roomId":13308358,"config":{"autoTranslate":false}}}));
```

### muvtuberdriver

```sh
cd muvtuberdriver
go run .
```

MuvtuberDriver 运行在 `:9010`:

```sh
curl -X POST localhost:9010 --data '{"author": "aaa", "content": "好难过呀"}'
```
