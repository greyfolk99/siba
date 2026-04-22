<!-- @doc payment-api -->
<!-- @const service-name = "payment-api" -->
<!-- @const version = "2.1.0" -->
<!-- @const base-url = "/v1/payments" -->

# Payment API

이 문서는 {{service-name}} v{{version}} 의 API 명세입니다.

## Endpoints

base URL: {{base-url}}

### Authentication
<!-- @let auth-type = "Bearer" -->
모든 요청에 {{auth-type}} token 필요.

### Routes

#### Create Payment
```
POST {{base-url}}
```

#### Get Payment
```
GET {{base-url}}/:id
```

## Error Handling

{{service-name}} 에러 코드:

| Code | Description |
|------|-------------|
| 400 | Bad request |
| 401 | 인증 실패 |
