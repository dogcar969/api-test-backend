# API测试平台

这是API测试平台的后端

[前端部分](https://github.com/dogcar969/api-test-frontend)

## 技术栈

gin

gorm

postgresql

golang-jwt

## 数据结构

User

- ID
- Name
- Password
- Requests
- Folders
  - ID
  - Name
  - Requests
    - ID
    - Parameters
      - ID
      - Type
      - Key
      - Value
      - RequestID
    - FolderID
    - UserID
    - Name
    - Url
    - Result
    - Method
    - ProtocolHeader
  - UserID

## 中间件

### 用户认证

使用jwt进行用户身份检测。

设计Authenticate中间件验证解析位于header["Authorization"]的jwt,并将用户id存储在上下文中，在需要用户认证的handler中使用。

使用exp字段，使jwt在设定的时间段后到期。

### 请求所有权验证、文件夹所有权验证

先根据请求的requestId/folderId获得对应的request/folder，如果有则根据UserID判断所有权，如果属于该用户则存储到上下文中。

## API

### 用户

**/login post**

用户登录，验证账号密码是否匹配，如果匹配生成jwt

**/user post**

用户注册

**/user patch**

修改用户名

**/user get**

获取用户信息，包含用户的所有文件夹的所有请求的详细信息。

### 文件夹

**/folder get**

获取文件夹以及文件夹的所有请求

**/folder post**

新建文件夹

**/folder patch**

文件夹重命名

**/folder delete**

删除文件夹以及文件夹的所有请求

### 请求

**/request get**

获取请求信息

**/request post**

新建请求

**/request patch**

修改请求，不包括外键和名字

**/request delete**

删除请求以及包含的参数

**/request/rename patch **

请求重命名

**/request/transfer patch**

请求在文件夹间转移（修改外键）