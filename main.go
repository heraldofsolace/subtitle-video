package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"

	"context"
	"io"
	"os"
	"bytes"
	"github.com/AssemblyAI/assemblyai-go-sdk"
	"github.com/joho/godotenv"
)

// The Job definition
type Job struct {
	Id string `json:"id"`
	FilePath string `json:"file_path"`
}

type Translation struct{
	Text string `json:"text"`
	DetectedSourceLanguage string `json:"detected_source_language"`
}

type TranslationResponse struct{
	Translations []Translation `json:"translations"`
}

// Empty jobs array
var jobs []Job

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	aaiApiKey := os.Getenv("AAI_API_KEY")

	client := assemblyai.NewClient(aaiApiKey)

    r := gin.Default()
	r.Use(ErrorHandler)

    r.LoadHTMLGlob("views/*.html")
	r.Static("/uploads", "uploads")

    r.GET("/", func(c *gin.Context) {
        c.HTML(http.StatusOK, "index.html", nil)
    })

	r.POST("/upload", func(c *gin.Context) {
		file, _ := c.FormFile("myFile")
		log.Println(file.Filename)

		c.SaveUploadedFile(file, "uploads/" + file.Filename)
		ctx := context.Background()
		f, err := os.Open("uploads/" + file.Filename)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		defer f.Close()
		transcript, err := client.Transcripts.SubmitFromReader(ctx, f, nil)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		id := *transcript.ID
		job := Job{Id: id, FilePath: "uploads/" + file.Filename}
		jobs = append(jobs, job)

		fmt.Printf("File %s uploaded successfully with ID %s\n", file.Filename, id)
		c.Redirect(http.StatusMovedPermanently, "/jobs/" + id)
	})
	
	r.GET("/jobs/:id", func(c *gin.Context) {
		id := c.Param("id")
		var job Job
		for _, j := range jobs {
			if j.Id == id {
				job = j
			}
		}
		c.HTML(http.StatusOK, "job.html", gin.H{
			"job": job,
		})
	})

	r.GET("/jobs/:id/status", func(c *gin.Context) {
		id := c.Param("id")
		var job Job
		for _, j := range jobs {
			if j.Id == id {
				job = j
			}
		}

		ctx := context.Background()
		transcript, err := client.Transcripts.Get(ctx, id)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		if transcript.Status == "processing" || transcript.Status == "queued" {
			c.JSON(http.StatusOK, gin.H{
				"status": "loading",
				"job": job,
			})
		} else if transcript.Status == "completed" {
			subtitles, err := client.Transcripts.GetSubtitles(ctx, id, "vtt", nil)
			if err != nil {
				c.AbortWithError(http.StatusInternalServerError, err)
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"status":    "completed",
				"job":       job,
				"subtitles": string(subtitles),
			})
		}
	})

	r.POST("/translate", func(c *gin.Context) {
		id := c.PostForm("job_id")
		language := c.PostForm("language")

		ctx := context.Background()
		subtitles, err := client.Transcripts.GetSubtitles(ctx, id, "vtt", nil)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		deepLApiKey := os.Getenv("DEEPL_API_KEY")
		body, _ := json.Marshal(map[string]any{
			"text": []string{string(subtitles)},
			"target_lang": language,
		})
		req, err := http.NewRequest("POST", "https://api-free.deepl.com/v2/translate", bytes.NewBuffer(body))
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "DeepL-Auth-Key " + deepLApiKey)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}

		var translationResponse TranslationResponse
		err = json.Unmarshal(respBody, &translationResponse)
		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, err)
			return
		}
		c.JSON(http.StatusOK, translationResponse)
	})

    r.Run(":3000")
}

func ErrorHandler(c *gin.Context) {
	c.Next()

	if(len(c.Errors) == 0) {
		return
	}

	for _, err := range c.Errors {
		log.Printf("Error: %s\n", err.Error())
	}

	c.JSON(http.StatusInternalServerError, "")
}