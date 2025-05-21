package main

import (
	"context"
	"encoding/json"
	_ "embed"
	"log"
	"os"
	"strings"

	"github.com/go-redis/redis/v8"
	"github.com/teris-io/shortid"
	"github.com/valyala/fasthttp"
)

//go:embed index.html
var indexHTML []byte

var (
	ctx     = context.Background()
	rdb     *redis.Client
	baseURL = "http://localhost:8080/" // Update if deployed
)

type RequestBody struct {
	Urls []string `json:"urls"`
}

type ResponseBody struct {
	Original string `json:"original"`
	ShortUrl string `json:"shortUrl"`
}

func main() {
	rdb = redis.NewClient(&redis.Options{
		Addr:     "redis:6379",
		Password: "",
		DB:       0,
	})

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Println("Connected to Redis")

	if err := fasthttp.ListenAndServe(":8080", routerHandler); err != nil {
		log.Fatalf("Error in ListenAndServe: %v", err)
	}
}

func enableCORS(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Access-Control-Allow-Origin", "*")
	ctx.Response.Header.Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
	ctx.Response.Header.Set("Access-Control-Allow-Headers", "Content-Type")
	if string(ctx.Method()) == fasthttp.MethodOptions {
		ctx.SetStatusCode(fasthttp.StatusNoContent)
	}
}

func routerHandler(ctx *fasthttp.RequestCtx) {
	enableCORS(ctx)

	path := string(ctx.Path())
	method := string(ctx.Method())

	if method == fasthttp.MethodOptions {
		return
	}

	switch {
	case path == "/" && method == fasthttp.MethodGet:
		serveIndex(ctx)
	case path == "/shorten" && method == fasthttp.MethodPost:
		shortenURLHandler(ctx)
	case strings.HasSuffix(path, ".png") || strings.HasSuffix(path, ".jpeg"):
		serveStaticImage(ctx, path[1:]) // Remove leading slash
	default:
		if method == fasthttp.MethodGet {
			redirectHandler(ctx)
			return
		}
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetBody([]byte("404 - Not Found"))
	}
}

func serveIndex(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBody(indexHTML)
}

func serveStaticImage(ctx *fasthttp.RequestCtx, filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetBody([]byte("Image not found"))
		return
	}
	if strings.HasSuffix(filename, ".png") {
		ctx.SetContentType("image/png")
	} else if strings.HasSuffix(filename, ".jpeg") {
		ctx.SetContentType("image/jpeg")
	}
	ctx.SetBody(data)
}

func shortenURLHandler(ctx *fasthttp.RequestCtx) {
	var reqBody RequestBody
	if err := json.Unmarshal(ctx.PostBody(), &reqBody); err != nil || len(reqBody.Urls) == 0 {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBody([]byte("Invalid or missing URLs"))
		return
	}

	var responses []ResponseBody

	for _, originalURL := range reqBody.Urls {
		id, err := findExistingID(originalURL)
		if err != nil {
			ctx.SetStatusCode(fasthttp.StatusInternalServerError)
			ctx.SetBody([]byte("Server error"))
			return
		}

		if id == "" {
			id, err = shortid.Generate()
			if err != nil {
				ctx.SetStatusCode(fasthttp.StatusInternalServerError)
				ctx.SetBody([]byte("Failed to generate ID"))
				return
			}

			if err := rdb.Set(ctx, id, originalURL, 0).Err(); err != nil {
				ctx.SetStatusCode(fasthttp.StatusInternalServerError)
				ctx.SetBody([]byte("Failed to save URL"))
				return
			}
		}

		responses = append(responses, ResponseBody{
			Original: originalURL,
			ShortUrl: baseURL + id,
		})
	}

	ctx.SetContentType("application/json")
	if err := json.NewEncoder(ctx).Encode(responses); err != nil {
		log.Printf("Failed to encode response: %v", err)
	}
}

func findExistingID(originalURL string) (string, error) {
	iter := rdb.Scan(ctx, 0, "*", 0).Iterator()
	for iter.Next(ctx) {
		id := iter.Val()
		val, err := rdb.Get(ctx, id).Result()
		if err != nil && err != redis.Nil {
			return "", err
		}
		if val == originalURL {
			return id, nil
		}
	}
	if err := iter.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func redirectHandler(ctx *fasthttp.RequestCtx) {
	id := string(ctx.Path())[1:]
	if id == "" {
		ctx.SetStatusCode(fasthttp.StatusBadRequest)
		ctx.SetBody([]byte("Missing short URL id"))
		return
	}

	originalURL, err := rdb.Get(ctx, id).Result()
	if err == redis.Nil {
		ctx.SetStatusCode(fasthttp.StatusNotFound)
		ctx.SetBody([]byte("Short URL not found"))
		return
	} else if err != nil {
		ctx.SetStatusCode(fasthttp.StatusInternalServerError)
		ctx.SetBody([]byte("Server error"))
		return
	}

	ctx.Redirect(originalURL, fasthttp.StatusMovedPermanently)
}
