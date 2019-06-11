package gcsstore_test

import (
	"bytes"
	"context"
	"testing"

	"gopkg.in/h2non/gock.v1"

	"cloud.google.com/go/storage"
	. "github.com/tus/tusd/pkg/gcsstore"
	"google.golang.org/api/option"
)

type googleObjectResponse struct {
	Name string `json:"name"`
}

type googleBucketResponse struct {
	Items []googleObjectResponse `json:"items"`
}

func TestGetObjectSize(t *testing.T) {
	defer gock.Off()

	gock.New("https://www.googleapis.com").
		Get("/storage/v1/b/test-bucket/o/test-name").
		MatchParam("alt", "json").
		MatchParam("projection", "full").
		Reply(200).
		JSON(map[string]string{"size": "54321"})

	gock.New("https://accounts.google.com/").
		Post("/o/oauth2/token").Reply(200).JSON(map[string]string{
		"access_token":  "H3l5321N123sdI4HLY/RF39FjrCRF39FjrCRF39FjrCRF39FjrC_RF39FjrCRF39FjrC",
		"token_type":    "Bearer",
		"refresh_token": "1/smWJksmWJksmWJksmWJksmWJk_smWJksmWJksmWJksmWJksmWJk",
		"expiry_date":   "1425333671141",
	})

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithAPIKey("foo"))
	if err != nil {
		t.Fatal(err)
		return
	}

	service := GCSService{
		Client: client,
	}

	size, err := service.GetObjectSize(ctx, GCSObjectParams{
		Bucket: "test-bucket",
		ID:     "test-name",
	})
	if err != nil {
		t.Errorf("Error: %v", err)
		return
	}

	if size != 54321 {
		t.Errorf("Error: Did not match given size")
		return
	}
}

func TestDeleteObjectWithFilter(t *testing.T) {
	defer gock.Off()

	gock.New("https://www.googleapis.com").
		Get("/storage/v1/b/test-bucket/o").
		MatchParam("alt", "json").
		MatchParam("pageToken", "").
		MatchParam("prefix", "test-prefix").
		MatchParam("projection", "full").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://accounts.google.com/").
		Post("/o/oauth2/token").Reply(200).JSON(map[string]string{
		"access_token":  "H3l5321N123sdI4HLY/RF39FjrCRF39FjrCRF39FjrCRF39FjrC_RF39FjrCRF39FjrC",
		"token_type":    "Bearer",
		"refresh_token": "1/smWJksmWJksmWJksmWJksmWJk_smWJksmWJksmWJksmWJksmWJk",
		"expiry_date":   "1425333671141",
	})

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithAPIKey("foo"))
	if err != nil {
		t.Fatal(err)
		return
	}

	service := GCSService{
		Client: client,
	}

	err = service.DeleteObjectsWithFilter(ctx, GCSFilterParams{
		Bucket: "test-bucket",
		Prefix: "test-prefix",
	})

	if err != nil {
		t.Errorf("Error: %v", err)
		return
	}

}

func TestComposeObjects(t *testing.T) {
	defer gock.Off()

	gock.New("https://www.googleapis.com").
		Get("/storage/v1/b/test-bucket/o/test1").
		MatchParam("alt", "json").
		MatchParam("projection", "full").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://www.googleapis.com").
		Get("/storage/v1/b/test-bucket/o/test2").
		MatchParam("alt", "json").
		MatchParam("projection", "full").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://www.googleapis.com").
		Get("/storage/v1/b/test-bucket/o/test3").
		MatchParam("alt", "json").
		MatchParam("projection", "full").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://www.googleapis.com").
		Get("/storage/v1/b/test-bucket/o/test1").
		MatchParam("alt", "json").
		MatchParam("projection", "full").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://www.googleapis.com").
		Post("/storage/v1/b/test-bucket/o/test_all/compose").
		MatchParam("alt", "json").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://www.googleapis.com").
		Get("/storage/v1/b/test-bucket/o/test_all").
		MatchParam("alt", "json").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://www.googleapis.com").
		Get("/storage/v1/b/test-bucket/o").
		MatchParam("alt", "json").
		MatchParam("delimiter", "").
		MatchParam("pageToken", "").
		MatchParam("prefix", "test_all_tmp").
		MatchParam("projection", "full").
		MatchParam("versions", "false").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://accounts.google.com/").
		Post("/o/oauth2/token").Reply(200).JSON(map[string]string{
		"access_token":  "H3l5321N123sdI4HLY/RF39FjrCRF39FjrCRF39FjrCRF39FjrC_RF39FjrCRF39FjrC",
		"token_type":    "Bearer",
		"refresh_token": "1/smWJksmWJksmWJksmWJksmWJk_smWJksmWJksmWJksmWJksmWJk",
		"expiry_date":   "1425333671141",
	})

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithAPIKey("foo"))
	if err != nil {
		t.Fatal(err)
		return
	}

	service := GCSService{
		Client: client,
	}

	err = service.ComposeObjects(ctx, GCSComposeParams{
		Bucket:      "test-bucket",
		Sources:     []string{"test1", "test2", "test3"},
		Destination: "test_all",
	})

	if err != nil {
		t.Errorf("Error: %v", err)
		return
	}
}

func TestGetObjectAttrs(t *testing.T) {
	defer gock.Off()

	gock.New("https://www.googleapis.com").
		Get("/storage/v1/b/test-bucket/o/test-name").
		MatchParam("alt", "json").
		MatchParam("projection", "full").
		Reply(200).
		JSON(map[string]string{"size": "54321", "name": "test_name"})

	gock.New("https://accounts.google.com/").
		Post("/o/oauth2/token").Reply(200).JSON(map[string]string{
		"access_token":  "H3l5321N123sdI4HLY/RF39FjrCRF39FjrCRF39FjrCRF39FjrC_RF39FjrCRF39FjrC",
		"token_type":    "Bearer",
		"refresh_token": "1/smWJksmWJksmWJksmWJksmWJk_smWJksmWJksmWJksmWJksmWJk",
		"expiry_date":   "1425333671141",
	})

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithAPIKey("foo"))
	if err != nil {
		t.Fatal(err)
		return
	}

	service := GCSService{
		Client: client,
	}

	attrs, err := service.GetObjectAttrs(ctx, GCSObjectParams{
		Bucket: "test-bucket",
		ID:     "test-name",
	})
	if err != nil {
		t.Errorf("Error: %v", err)
		return
	}

	if attrs.Name != "test_name" && attrs.Size != 54321 {
		t.Errorf("Mismatched attributes - got: %+v", attrs)
		return
	}

}

func TestReadObject(t *testing.T) {
	defer gock.Off()

	gock.New("https://storage.googleapis.com").
		Get("/test-bucket/test-name").
		Reply(200).
		JSON(map[string]string{"this": "is", "a": "fake file"})

	gock.New("https://accounts.google.com/").
		Post("/o/oauth2/token").Reply(200).JSON(map[string]string{
		"access_token":  "H3l5321N123sdI4HLY/RF39FjrCRF39FjrCRF39FjrCRF39FjrC_RF39FjrCRF39FjrC",
		"token_type":    "Bearer",
		"refresh_token": "1/smWJksmWJksmWJksmWJksmWJk_smWJksmWJksmWJksmWJksmWJk",
		"expiry_date":   "1425333671141",
	})

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithAPIKey("foo"))
	if err != nil {
		t.Fatal(err)
		return
	}

	service := GCSService{
		Client: client,
	}

	reader, err := service.ReadObject(ctx, GCSObjectParams{
		Bucket: "test-bucket",
		ID:     "test-name",
	})

	if reader.Size() != 30 {
		t.Errorf("Object size does not match expected value: %+v", reader)
	}
}

func TestSetObjectMetadata(t *testing.T) {
	defer gock.Off()

	gock.New("https://googleapis.com").
		Patch("/storage/v1/b/test-bucket/o/test-name").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://accounts.google.com/").
		Post("/o/oauth2/token").Reply(200).JSON(map[string]string{
		"access_token":  "H3l5321N123sdI4HLY/RF39FjrCRF39FjrCRF39FjrCRF39FjrC_RF39FjrCRF39FjrC",
		"token_type":    "Bearer",
		"refresh_token": "1/smWJksmWJksmWJksmWJksmWJk_smWJksmWJksmWJksmWJksmWJk",
		"expiry_date":   "1425333671141",
	})

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithAPIKey("foo"))
	if err != nil {
		t.Fatal(err)
		return
	}

	service := GCSService{
		Client: client,
	}

	err = service.SetObjectMetadata(ctx, GCSObjectParams{
		Bucket: "test-bucket",
		ID:     "test-name",
	}, map[string]string{"test": "metadata", "fake": "test"})

	if err != nil {
		t.Errorf("Error updating metadata: %+v", err)
	}
}

func TestDeleteObject(t *testing.T) {
	defer gock.Off()

	gock.New("https://googleapis.com").
		Delete("/storage/v1/b/test-bucket/o/test-name").
		MatchParam("alt", "json").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://accounts.google.com/").
		Post("/o/oauth2/token").Reply(200).JSON(map[string]string{
		"access_token":  "H3l5321N123sdI4HLY/RF39FjrCRF39FjrCRF39FjrCRF39FjrC_RF39FjrCRF39FjrC",
		"token_type":    "Bearer",
		"refresh_token": "1/smWJksmWJksmWJksmWJksmWJk_smWJksmWJksmWJksmWJksmWJk",
		"expiry_date":   "1425333671141",
	})

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithAPIKey("foo"))
	if err != nil {
		t.Fatal(err)
		return
	}

	service := GCSService{
		Client: client,
	}

	err = service.DeleteObject(ctx, GCSObjectParams{
		Bucket: "test-bucket",
		ID:     "test-name",
	})

	if err != nil {
		t.Errorf("Error deleting object: %+v", err)
	}
}

func TestWriteObject(t *testing.T) {
	defer gock.Off()

	gock.New("https://accounts.google.com/").
		Post("/o/oauth2/token").Reply(200).JSON(map[string]string{
		"access_token":  "H3l5321N123sdI4HLY/RF39FjrCRF39FjrCRF39FjrCRF39FjrC_RF39FjrCRF39FjrC",
		"token_type":    "Bearer",
		"refresh_token": "1/smWJksmWJksmWJksmWJksmWJk_smWJksmWJksmWJksmWJksmWJk",
		"expiry_date":   "1425333671141",
	})

	gock.New("https://googleapis.com").
		Post("/upload/storage/v1/b/test-bucket/o").
		MatchParam("alt", "json").
		MatchParam("key", "foo").
		Reply(200).
		JSON(map[string]string{})

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithAPIKey("foo"))
	if err != nil {
		t.Fatal(err)
		return
	}

	service := GCSService{
		Client: client,
	}

	reader := bytes.NewReader([]byte{1})

	size, err := service.WriteObject(ctx, GCSObjectParams{
		Bucket: "test-bucket",
		ID:     "test-name",
	}, reader)

	if err != nil {
		t.Errorf("Error writing object: %+v", err)
	}

	if size != 1 {
		t.Errorf("Mismatch of object size: %v", size)
	}
}

func TestComposeFrom(t *testing.T) {
	defer gock.Off()

	gock.New("https://googleapis.com").
		Post("/storage/v1/b/test-bucket/o/my-object/compose").
		MatchParam("alt", "json").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://googleapis.com").
		Get("/storage/v1/b/test-bucket/o/my-object").
		MatchParam("alt", "json").
		MatchParam("projection", "full").
		Reply(200).
		JSON(map[string]string{})

	gock.New("https://accounts.google.com/").
		Post("/o/oauth2/token").Reply(200).JSON(map[string]string{
		"access_token":  "H3l5321N123sdI4HLY/RF39FjrCRF39FjrCRF39FjrCRF39FjrC_RF39FjrCRF39FjrC",
		"token_type":    "Bearer",
		"refresh_token": "1/smWJksmWJksmWJksmWJksmWJk_smWJksmWJksmWJksmWJksmWJk",
		"expiry_date":   "1425333671141",
	})

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithAPIKey("foo"))
	if err != nil {
		t.Fatal(err)
		return
	}

	service := GCSService{
		Client: client,
	}

	crc, err := service.ComposeFrom(ctx, []*storage.ObjectHandle{client.Bucket("test-bucket").Object("my-object")}, GCSObjectParams{
		Bucket: "test-bucket",
		ID:     "my-object",
	}, "text")

	if err != nil {
		t.Errorf("Error composing multiple objects: %+v", err)
	}

	if crc != 0 {
		t.Errorf("Error composing multiple objects: %v", crc)
	}
}

func TestFilterObject(t *testing.T) {
	defer gock.Off()

	resp := googleBucketResponse{[]googleObjectResponse{
		googleObjectResponse{Name: "test-prefix_1"},
	}}

	gock.New("https://www.googleapis.com").
		Get("/storage/v1/b/test-bucket/o").
		MatchParam("alt", "json").
		MatchParam("pageToken", "").
		MatchParam("prefix", "test-prefix").
		MatchParam("projection", "full").
		Reply(200).
		JSON(resp)

	gock.New("https://accounts.google.com/").
		Post("/o/oauth2/token").Reply(200).JSON(map[string]string{
		"access_token":  "H3l5321N123sdI4HLY/RF39FjrCRF39FjrCRF39FjrCRF39FjrC_RF39FjrCRF39FjrC",
		"token_type":    "Bearer",
		"refresh_token": "1/smWJksmWJksmWJksmWJksmWJk_smWJksmWJksmWJksmWJksmWJk",
		"expiry_date":   "1425333671141",
	})

	ctx := context.Background()
	client, err := storage.NewClient(ctx, option.WithAPIKey("foo"))
	if err != nil {
		t.Fatal(err)
		return
	}

	service := GCSService{
		Client: client,
	}

	objects, err := service.FilterObjects(ctx, GCSFilterParams{
		Bucket: "test-bucket",
		Prefix: "test-prefix",
	})

	if err != nil {
		t.Errorf("Error: %v", err)
		return
	}

	if len(objects) != 2 {
		t.Errorf("Didn't get appropriate amount of objects back: got %v from %v", len(objects), objects)
	}
}
