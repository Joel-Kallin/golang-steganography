package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/google/uuid"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"
)

type Body struct {
	Extension string `json:"extension"`
	File string `json:"file"`
}

var errorLogger = log.New(os.Stderr, "ERROR ", log.Llongfile)

var infoLogger = log.New(os.Stderr, "INFO ", log.Llongfile)

type resolver func(events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error)

var encodeHttpMethodMap = map[string]resolver{
	"GET": encode,
}

var decodeHttpMethodMap = map[string]resolver{
	"GET": decode,
}

var uploadHttpMethodMap = map[string]resolver{
	"POST": upload,
}

var maxRequestBodySize = int64(8000000)

func upload(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	bodyRequest := Body{
		Extension: "",
		File: "",
	}

	err := json.Unmarshal([]byte(req.Body), &bodyRequest)

	if err != nil {
		return serverError(err)
	}

	file, err := base64.StdEncoding.DecodeString(bodyRequest.File)

	if err != nil {
		return clientError(http.StatusBadRequest)
	}

	url, err := uploadFileToS3(file, bodyRequest.Extension)

	if err != nil {
		return serverError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{"Access-Control-Allow-Origin":"*","Content-Type": "application/json"},
		Body:       url,
	}, nil
}

func uploadFileToS3(file []byte, extension string) (string, error) {
	bucketName := "stegonography"
	fileName := uuid.New().String()+"."+extension
	sess := session.Must(session.NewSession())
	uploader := s3manager.NewUploader(sess)
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(fileName),
		Body:   bytes.NewReader(file),
	})

	if err != nil {
		return "", err
	}

	svc := s3.New(sess)

	req, _ := svc.GetObjectRequest(&s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(fileName),
	})
	return req.Presign(24 * time.Hour)
}

func downloadFileFromUrl(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

func serverError(err error) (events.APIGatewayProxyResponse, error) {
	errorLogger.Println(err.Error())

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusInternalServerError,
		Body:       http.StatusText(http.StatusInternalServerError),
	}, nil
}

func clientError(status int) (events.APIGatewayProxyResponse, error) {
	return clientErrorWithMessage(status, "")
}

func clientErrorWithMessage(status int, message string) (events.APIGatewayProxyResponse, error) {
	infoLogger.Println(message)
	body := http.StatusText(status)
	if len(message)>0 {
		body = body + " : "+message
	}
	return events.APIGatewayProxyResponse{
		StatusCode: status,
		Body:       body,
	}, nil
}


func router(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	switch req.Path {
	case "/api/encode":
		return routeHttpMethod(req, encodeHttpMethodMap)
	case "/api/decode":
		return routeHttpMethod(req, decodeHttpMethodMap)
	case "/api/upload":
		return routeHttpMethod(req, uploadHttpMethodMap)
	default:
		return clientErrorWithMessage(http.StatusMethodNotAllowed, req.Path)
	}
}

func routeHttpMethod(req events.APIGatewayProxyRequest, httpMethodMap map[string]resolver) (events.APIGatewayProxyResponse, error) {

	handler, exists := httpMethodMap[req.HTTPMethod]

	if exists {
		return handler(req)
	}

	return clientErrorWithMessage(http.StatusMethodNotAllowed, "handler for method does not exist")
}

func encode(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	secretMessage := req.QueryStringParameters["message"]

	imageUrl, isValid := validImageQueryParams(req)

	if len(secretMessage)==0 || !isValid {
		return clientError(http.StatusBadRequest)
	}

	extension := getExtensionFromLink(imageUrl)

	if len(extension)==0 {
		errorLogger.Println("unknown file extension")
		return clientError(http.StatusBadRequest)
	}

	file, err := downloadFileFromUrl(imageUrl)

	if err != nil {
		errorLogger.Println("error downloading file")
		return serverError(err)
	}

	decodedMessage, err := url.QueryUnescape(secretMessage)

	if err != nil {
		errorLogger.Println("error encoding file")
		return clientError(http.StatusBadRequest)
	}

	encodedFile, err := Encode(file, []byte(decodedMessage))

	if err != nil {
		errorLogger.Println("error encoding file")
		return serverError(err)
	}

	url, err := uploadFileToS3(encodedFile, extension)

	if err != nil {
		errorLogger.Println("error uploading file")
		return serverError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{"content-type":"application/json", "charset":"utf-8"},
		Body:       url,
	}, nil
}

func getExtensionFromLink(url string) string {
	byteUrl := []byte(url)
	re := regexp.MustCompile(`(jpeg|jpg|png|img)`)

	return string(re.Find(byteUrl))
}

func validImageQueryParams(req events.APIGatewayProxyRequest) (string, bool) {

	s3ObjectName := req.QueryStringParameters["s3"]

	url := req.QueryStringParameters["url"]

	if s3ObjectName=="" && url=="" {
		return "", false
	}

	if len(s3ObjectName) > 0 {
		decoded, err := base64.StdEncoding.DecodeString(s3ObjectName)
		if err != nil {
			return "", false
		}
		return string(decoded), true
	}

	decoded, err := base64.StdEncoding.DecodeString(url)
	if err != nil {
		return "", false
	}
	return string(decoded), true
}

func decode(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	imageUrl, isValid := validImageQueryParams(req)

	if !isValid {
		return clientError(http.StatusBadRequest)
	}

	var file []byte
	var err error

	file, err = downloadFileFromUrl(imageUrl)

	if err != nil {
		errorLogger.Println("error downloading file")
		return serverError(err)
	}

	msg, err := Decode(file)

	if err != nil {
		errorLogger.Println("error decoding file")
		return serverError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{"content-type":"application/json", "charset":"utf-8"},
		Body:       msg,
	}, nil

}

func main() {
	lambda.Start(router)
}
