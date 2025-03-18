#### milcon

`milcon` is a PoC server that handles as many connections as possible simultaneously, using the unix epoll mechanism.
It is heavily inspired by [1m-websockets](https://github.com/eranyanay/1m-go-websockets/tree/master).
The extra mechanism is that it tries to let the server send packets to the client, utilising the existing connections, and not just wait for the client to trigger the epoll I/O event.

##### run
server:
```
make rs
````

client:
```
make rc CONN_NUM=40000 WS_URL=ws://192.168.60.142:6767/ws
```
