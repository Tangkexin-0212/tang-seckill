namespace go user

struct User {
    1: i64 id
    2: string username
    3: string nickname
    4: i64 createdAtMs
}

struct LoginResp {
    1: string token
    2: i64 uid
    3: i64 expireAtMs
}

service UserService {
    // 注册，返回 uid
    i64 Register(1: string username, 2: string password)
    // 登录，返回 token（存 Redis）
    LoginResp Login(1: string username, 2: string password)
    // 查询用户
    User GetUser(1: i64 uid)
    // 网关鉴权：token -> uid，失败返回 0
    i64 VerifyToken(1: string token)
}
