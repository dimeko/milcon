#### milcon

`milcon` is a PoC serve that handles as many connections as possible simultaneously. It uses the unix epoll mechanism. It is heavily inspired [1m-websockets](https://github.com/eranyanay/1m-go-websockets/tree/master) but it tries to make the server more independent and not wait for client to send a message and then respond but send messages to clients on events triggered by the server itself always utilising epoll.

##### run
server:
```
make rs
````

client:
```
make rc CONN_NUM=40000 WS_URL=ws://192.168.60.142:6767/ws
```