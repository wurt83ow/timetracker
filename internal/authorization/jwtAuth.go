package authz

import (
    "context"
    "crypto/sha256"
    "errors"
    "log"
    "net/http"

    "github.com/golang-jwt/jwt"
    "github.com/wurt83ow/timetracker/internal/config"
    "github.com/wurt83ow/timetracker/internal/models"
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

type CustomClaims struct {
    PassportNumber string `json:"passport_number"`
    jwt.StandardClaims
}

type Log interface {
    Info(string, ...zapcore.Field)
}

type JWTAuthz struct {
    jwtSigningKey    []byte
    log              Log
    jwtSigningMethod *jwt.SigningMethodHMAC
    defaultCookie    http.Cookie
}

func NewJWTAuthz(signingKey string, log Log) *JWTAuthz {
    return &JWTAuthz{
        jwtSigningKey:    []byte(config.GetAsString("JWT_SIGNING_KEY", signingKey)),
        log:              log,
        jwtSigningMethod: jwt.SigningMethodHS256,

        defaultCookie: http.Cookie{
            HttpOnly: true,
            // SameSite: http.SameSiteLaxMode,
            // Domain:   configs.GetAsString("COOKIE_DOMAIN", "localhost"),
            // Secure:   configs.GetAsBool("COOKIE_SECURE", true),
        },
    }
}

// JWTAuthzMiddleware verifies a valid JWT exists in our
// cookie and if not, encourages the consumer to login again.
func (j *JWTAuthz) JWTAuthzMiddleware(log Log) func(next http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        fn := func(w http.ResponseWriter, r *http.Request) {
            // Grab jwt-token cookie
            jwtCookie, err := r.Cookie("jwt-token")

            var userID string
            if err == nil {
                userID, err = j.DecodeJWTToUser(jwtCookie.Value)
                if err != nil {
                    userID = ""
                    log.Info("Error occurred creating a cookie", zap.Error(err))
                }
            } else {
                log.Info("Error occurred reading cookie", zap.Error(err))
            }

            if userID == "" {
                jwtHeader := r.Header.Get("Authorization")

                if jwtHeader != "" {
                    userID, err = j.DecodeJWTToUser(jwtHeader)
                    if err != nil {
                        userID = ""
                        log.Info("Error occurred decoding token from header", zap.Error(err))
                    }
                }
            }

            if userID == "" {
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }

            var keyUserID models.Key = "userID"
            ctx := r.Context()
            ctx = context.WithValue(ctx, keyUserID, userID)

            next.ServeHTTP(w, r.WithContext(ctx))
        }

        return http.HandlerFunc(fn)
    }
}

func (j *JWTAuthz) CreateJWTTokenForUser(userid string) string {
    claims := CustomClaims{
        PassportNumber: userid,
        StandardClaims: jwt.StandardClaims{},
    }

    // Encode to token string
    tokenString, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(j.jwtSigningKey)
    if err != nil {
        log.Println("Error occurred generating JWT", err)
        return ""
    }

    return tokenString
}

func (j *JWTAuthz) DecodeJWTToUser(token string) (string, error) {
    // Decode
    decodedToken, err := jwt.ParseWithClaims(token, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
        if !(j.jwtSigningMethod == token.Method) {
            // Check our method hasn't changed since issuance
            return nil, errors.New("signing method mismatch")
        }

        return j.jwtSigningKey, nil
    })

    // There's two parts. We might decode it successfully but it might
    // be the case we aren't Valid so you must check both
    if decClaims, ok := decodedToken.Claims.(*CustomClaims); ok && decodedToken.Valid {
        return decClaims.PassportNumber, nil
    }

    return "", err
}

func (j *JWTAuthz) GetHash(passportNumber string, password string) []byte {
    src := []byte(passportNumber + password)

    // create a new hash.Hash that calculates the SHA-256 checksum
    h := sha256.New()
    // transfer bytes for hashing
    h.Write(src)
    // calculate the hash

    return h.Sum(nil)
}

func (j *JWTAuthz) AuthCookie(name string, token string) *http.Cookie {
    d := j.defaultCookie
    d.Name = name
    d.Value = token
    d.Path = "/"

    return &d
}
