package s3

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"harborshield/backend/internal/audit"
	"harborshield/backend/internal/authz"
	"harborshield/backend/internal/buckets"
	"harborshield/backend/internal/credentials"
	"harborshield/backend/internal/events"
	metricspkg "harborshield/backend/internal/metrics"
	"harborshield/backend/internal/multipart"
	"harborshield/backend/internal/objects"
	"harborshield/backend/internal/policies"
	"harborshield/backend/internal/quotas"
)

type RouterDeps struct {
	Buckets            *buckets.Service
	Objects            *objects.Service
	Multipart          *multipart.Service
	Credentials        *credentials.Service
	Audit              *audit.Service
	Events             *events.Service
	Authorizer         *authz.Authorizer
	Policies           *policies.Service
	Quotas             *quotas.Service
	Metrics            *metricspkg.Registry
	PresignTTL         time.Duration
	MaxUploadSizeBytes int64
	Region             string
}

type bucketListResponse struct {
	XMLName xml.Name `xml:"ListAllMyBucketsResult"`
	Buckets []struct {
		Name         string `xml:"Name"`
		CreationDate string `xml:"CreationDate"`
	} `xml:"Buckets>Bucket"`
}

type objectListResponse struct {
	XMLName  xml.Name `xml:"ListBucketResult"`
	Name     string   `xml:"Name"`
	Contents []struct {
		Key          string `xml:"Key"`
		Size         int64  `xml:"Size"`
		LastModified string `xml:"LastModified"`
		ETag         string `xml:"ETag"`
	} `xml:"Contents"`
}

type listObjectsV2Response struct {
	XMLName               xml.Name `xml:"ListBucketResult"`
	Name                  string   `xml:"Name"`
	Prefix                string   `xml:"Prefix,omitempty"`
	KeyCount              int      `xml:"KeyCount"`
	MaxKeys               int      `xml:"MaxKeys"`
	IsTruncated           bool     `xml:"IsTruncated"`
	ContinuationToken     string   `xml:"ContinuationToken,omitempty"`
	NextContinuationToken string   `xml:"NextContinuationToken,omitempty"`
	StartAfter            string   `xml:"StartAfter,omitempty"`
	Contents              []struct {
		Key          string `xml:"Key"`
		Size         int64  `xml:"Size"`
		LastModified string `xml:"LastModified"`
		ETag         string `xml:"ETag"`
	} `xml:"Contents"`
}

type versionListResponse struct {
	XMLName       xml.Name       `xml:"ListVersionsResult"`
	Name          string         `xml:"Name"`
	Prefix        string         `xml:"Prefix,omitempty"`
	Versions      []versionEntry `xml:"Version"`
	DeleteMarkers []deleteMarker `xml:"DeleteMarker"`
}

type versionEntry struct {
	Key          string `xml:"Key"`
	VersionID    string `xml:"VersionId"`
	IsLatest     bool   `xml:"IsLatest"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
}

type deleteMarker struct {
	Key          string `xml:"Key"`
	VersionID    string `xml:"VersionId"`
	IsLatest     bool   `xml:"IsLatest"`
	LastModified string `xml:"LastModified"`
}

type initiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

type completeMultipartUploadResult struct {
	XMLName xml.Name `xml:"CompleteMultipartUploadResult"`
	Bucket  string   `xml:"Bucket"`
	Key     string   `xml:"Key"`
	ETag    string   `xml:"ETag"`
}

type copyObjectResult struct {
	XMLName      xml.Name `xml:"CopyObjectResult"`
	LastModified string   `xml:"LastModified"`
	ETag         string   `xml:"ETag"`
}

type taggingResponse struct {
	XMLName xml.Name  `xml:"Tagging"`
	TagSet  []tagPair `xml:"TagSet>Tag"`
}

type tagPair struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

func Mount(r chi.Router, deps RouterDeps) {
	r.Route("/s3", func(r chi.Router) {
		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			if _, status := authorizeS3Request(req, deps, "bucket.list", "*"); status != authAllowed {
				writeS3AuthorizationError(w, req, status)
				return
			}
			items, err := deps.Buckets.List(req.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			resp := bucketListResponse{}
			for _, item := range items {
				resp.Buckets = append(resp.Buckets, struct {
					Name         string `xml:"Name"`
					CreationDate string `xml:"CreationDate"`
				}{Name: item.Name, CreationDate: item.CreatedAt})
			}
			w.Header().Set("Content-Type", "application/xml")
			_ = xml.NewEncoder(w).Encode(resp)
		})

		r.Put("/{bucket}", func(w http.ResponseWriter, req *http.Request) {
			bucketName := chi.URLParam(req, "bucket")
			if _, ok := req.URL.Query()["policy"]; ok {
				actor, status := authorizeS3Request(req, deps, "bucket.policy.put", bucketResource(bucketName))
				if status != authAllowed {
					writeS3AuthorizationError(w, req, status)
					return
				}
				bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
				if err != nil {
					writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
					return
				}
				document, err := io.ReadAll(req.Body)
				if err != nil {
					writeError(w, req, http.StatusBadRequest, "MalformedPolicy", "The bucket policy body could not be read.")
					return
				}
				if _, err := deps.Policies.PutBucketPolicy(req.Context(), bucket.ID, document); err != nil {
					writeError(w, req, http.StatusBadRequest, "MalformedPolicy", "The bucket policy must be valid AWS-style JSON.")
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     actor["userID"],
					Action:    "bucket.policy.put",
					Resource:  bucketName,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
				})
				w.WriteHeader(http.StatusNoContent)
				return
			}
			actor, status := authorizeS3Request(req, deps, "bucket.create", bucketResource(bucketName))
			if status != authAllowed {
				writeS3AuthorizationError(w, req, status)
				return
			}
			item, err := deps.Buckets.Create(req.Context(), bucketName, "default", actor["userID"], "inherit", 0)
			if err != nil {
				writeError(w, req, http.StatusBadRequest, "InvalidBucketName", err.Error())
				return
			}
			_ = deps.Audit.Record(req.Context(), audit.Entry{
				Actor:     actor["userID"],
				Action:    "bucket.create",
				Resource:  bucketName,
				Outcome:   "success",
				RequestID: req.Header.Get("X-Request-Id"),
				Detail:    map[string]any{"bucketId": item.ID},
			})
			_ = deps.Events.Emit(req.Context(), "bucket.created", map[string]any{"bucketId": item.ID, "bucketName": item.Name})
			w.WriteHeader(http.StatusOK)
		})

		r.Delete("/{bucket}", func(w http.ResponseWriter, req *http.Request) {
			bucketName := chi.URLParam(req, "bucket")
			if _, ok := req.URL.Query()["policy"]; ok {
				actor, status := authorizeS3Request(req, deps, "bucket.policy.delete", bucketResource(bucketName))
				if status != authAllowed {
					writeS3AuthorizationError(w, req, status)
					return
				}
				bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
				if err != nil {
					writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
					return
				}
				if err := deps.Policies.DeleteBucketPolicy(req.Context(), bucket.ID); err != nil {
					if errors.Is(err, policies.ErrBucketPolicyNotFound) {
						writeError(w, req, http.StatusNotFound, "NoSuchBucketPolicy", "The bucket policy does not exist.")
						return
					}
					writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     actor["userID"],
					Action:    "bucket.policy.delete",
					Resource:  bucketName,
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
				})
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if _, status := authorizeS3Request(req, deps, "bucket.delete", bucketResource(bucketName)); status != authAllowed {
				writeS3AuthorizationError(w, req, status)
				return
			}
			if err := deps.Buckets.Delete(req.Context(), bucketName); err != nil {
				switch {
				case errors.Is(err, buckets.ErrBucketNotEmpty):
					writeError(w, req, http.StatusConflict, "BucketNotEmpty", "The bucket you tried to delete is not empty.")
				case errors.Is(err, buckets.ErrBucketNotFound):
					writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
				default:
					writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
				}
				return
			}
			_ = deps.Events.Emit(req.Context(), "bucket.deleted", map[string]any{"bucketName": bucketName})
			w.WriteHeader(http.StatusNoContent)
		})

		r.Get("/{bucket}", func(w http.ResponseWriter, req *http.Request) {
			bucketName := chi.URLParam(req, "bucket")
			bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
			if err != nil {
				writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
				return
			}
			if _, ok := req.URL.Query()["policy"]; ok {
				if _, status := authorizeS3Request(req, deps, "bucket.policy.get", bucketResource(bucketName)); status != authAllowed {
					writeS3AuthorizationError(w, req, status)
					return
				}
				document, err := deps.Policies.GetBucketPolicy(req.Context(), bucket.ID)
				if err != nil {
					if errors.Is(err, policies.ErrBucketPolicyNotFound) {
						writeError(w, req, http.StatusNotFound, "NoSuchBucketPolicy", "The bucket policy does not exist.")
						return
					}
					writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(document)
				return
			}
			if _, ok := req.URL.Query()["versions"]; ok {
				prefix := req.URL.Query().Get("prefix")
				if _, status := authorizeBucketPolicyAware(req, deps, bucket.ID, "", "object.list", bucketResource(bucketName), "s3:ListBucketVersions", bucketPolicyResource(bucketName, "")); status != authAllowed {
					writeS3AuthorizationError(w, req, status)
					return
				}
				items, err := deps.Objects.ListAllVersions(req.Context(), bucket.ID, prefix)
				if err != nil {
					writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
					return
				}
				resp := versionListResponse{Name: bucket.Name, Prefix: prefix}
				for _, item := range items {
					if item.IsDeleteMarker {
						resp.DeleteMarkers = append(resp.DeleteMarkers, deleteMarker{
							Key:          item.Key,
							VersionID:    item.VersionID,
							IsLatest:     item.IsLatest,
							LastModified: item.CreatedAt,
						})
						continue
					}
					resp.Versions = append(resp.Versions, versionEntry{
						Key:          item.Key,
						VersionID:    item.VersionID,
						IsLatest:     item.IsLatest,
						LastModified: item.CreatedAt,
						ETag:         quoteETag(item.ETag),
						Size:         item.SizeBytes,
					})
				}
				w.Header().Set("Content-Type", "application/xml")
				_ = xml.NewEncoder(w).Encode(resp)
				return
			}
			if _, status := authorizeBucketPolicyAware(req, deps, bucket.ID, "", "object.list", bucketResource(bucketName), "s3:ListBucket", bucketPolicyResource(bucketName, "")); status != authAllowed {
				writeS3AuthorizationError(w, req, status)
				return
			}
			prefix := req.URL.Query().Get("prefix")
			items, err := deps.Objects.List(req.Context(), bucket.ID, prefix)
			if err != nil {
				writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
				return
			}
			if req.URL.Query().Get("list-type") == "2" {
				resp := buildListObjectsV2Response(bucket.Name, prefix, req.URL.Query(), items)
				w.Header().Set("Content-Type", "application/xml")
				_ = xml.NewEncoder(w).Encode(resp)
				return
			}
			resp := objectListResponse{Name: bucket.Name}
			for _, item := range items {
				resp.Contents = append(resp.Contents, struct {
					Key          string `xml:"Key"`
					Size         int64  `xml:"Size"`
					LastModified string `xml:"LastModified"`
					ETag         string `xml:"ETag"`
				}{Key: item.Key, Size: item.SizeBytes, LastModified: item.CreatedAt, ETag: quoteETag(item.ETag)})
			}
			w.Header().Set("Content-Type", "application/xml")
			_ = xml.NewEncoder(w).Encode(resp)
		})

		r.Put("/{bucket}/*", func(w http.ResponseWriter, req *http.Request) {
			bucketName := chi.URLParam(req, "bucket")
			key := chi.URLParam(req, "*")
			if _, ok := req.URL.Query()["tagging"]; ok {
				bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
				if err != nil {
					writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
					return
				}
				actor, status := authorizeBucketPolicyAware(req, deps, bucket.ID, key, "object.put", objectResource(bucketName, key), "s3:PutObjectTagging", bucketPolicyResource(bucketName, key))
				if status != authAllowed {
					writeS3AuthorizationError(w, req, status)
					return
				}
				tags, err := parseTaggingXML(req.Body)
				if err != nil {
					writeError(w, req, http.StatusBadRequest, "InvalidTag", "The tagging payload is invalid.")
					return
				}
				versionID := req.URL.Query().Get("versionId")
				item, err := deps.Objects.PutTags(req.Context(), bucket.ID, key, versionID, tags)
				if err != nil {
					writeError(w, req, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     actor["userID"],
					Action:    "object.tagging.put",
					Resource:  fmt.Sprintf("%s/%s", bucket.Name, key),
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"bucketId": bucket.ID, "versionId": item.VersionID, "tagCount": len(item.Tags)},
				})
				w.WriteHeader(http.StatusOK)
				return
			}
			if uploadID := req.URL.Query().Get("uploadId"); uploadID != "" {
				bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
				if err != nil {
					writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
					return
				}
				if _, status := authorizeBucketPolicyAware(req, deps, bucket.ID, key, "object.put", objectResource(bucketName, key), "s3:PutObject", bucketPolicyResource(bucketName, key)); status != authAllowed {
					writeS3AuthorizationError(w, req, status)
					return
				}
				partNumber, err := strconv.Atoi(req.URL.Query().Get("partNumber"))
				if err != nil || partNumber <= 0 {
					writeError(w, req, http.StatusBadRequest, "InvalidPart", "The requested multipart part number is invalid.")
					return
				}
				if req.ContentLength > deps.MaxUploadSizeBytes {
					writeError(w, req, http.StatusRequestEntityTooLarge, "EntityTooLarge", "The proposed upload exceeds the configured upload size limit.")
					return
				}
				req.Body = http.MaxBytesReader(w, req.Body, deps.MaxUploadSizeBytes)
				part, err := deps.Multipart.PutPart(req.Context(), uploadID, partNumber, expectedContentLength(req), req.Header.Get("Content-MD5"), req.Body)
				if err != nil {
					if strings.Contains(err.Error(), "http: request body too large") {
						writeError(w, req, http.StatusRequestEntityTooLarge, "EntityTooLarge", "The proposed upload exceeds the configured upload size limit.")
						return
					}
					switch {
					case errors.Is(err, objects.ErrIncompleteBody):
						writeError(w, req, http.StatusBadRequest, "IncompleteBody", "You did not provide the number of bytes specified by the Content-Length HTTP header.")
					case errors.Is(err, objects.ErrInvalidDigest):
						writeError(w, req, http.StatusBadRequest, "InvalidDigest", "The Content-MD5 you specified was invalid.")
					case errors.Is(err, objects.ErrBadDigest):
						writeError(w, req, http.StatusBadRequest, "BadDigest", "The Content-MD5 you specified did not match what we received.")
					default:
						writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
					}
					return
				}
				w.Header().Set("ETag", quoteETag(part.ETag))
				w.WriteHeader(http.StatusOK)
				return
			}
			bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
			if err != nil {
				writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
				return
			}
			actor, status := authorizeBucketPolicyAware(req, deps, bucket.ID, key, "object.put", objectResource(bucketName, key), "s3:PutObject", bucketPolicyResource(bucketName, key))
			if status != authAllowed {
				writeS3AuthorizationError(w, req, status)
				return
			}
			if req.ContentLength > deps.MaxUploadSizeBytes {
				writeError(w, req, http.StatusRequestEntityTooLarge, "EntityTooLarge", "The proposed upload exceeds the configured upload size limit.")
				return
			}
			req.Body = http.MaxBytesReader(w, req.Body, deps.MaxUploadSizeBytes)
			createdBy := ""
			if actor["userID"] != "" {
				createdBy = actor["userID"]
			}
			copySource := req.Header.Get("X-Amz-Copy-Source")
			var item objects.Metadata
			if copySource != "" {
				sourceBucketName, sourceKey, sourceVersionID, err := parseCopySource(copySource)
				if err != nil {
					writeError(w, req, http.StatusBadRequest, "InvalidRequest", "The copy source header is invalid.")
					return
				}
				sourceBucket, err := deps.Buckets.GetByName(req.Context(), sourceBucketName)
				if err != nil {
					writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified source bucket does not exist.")
					return
				}
				if _, sourceStatus := authorizeS3Request(req, deps, "object.get", objectResource(sourceBucketName, sourceKey)); sourceStatus != authAllowed {
					writeS3AuthorizationError(w, req, sourceStatus)
					return
				}
				userMetadata := map[string]string(nil)
				if strings.EqualFold(req.Header.Get("X-Amz-Metadata-Directive"), "REPLACE") {
					userMetadata = extractUserMetadata(req.Header)
				}
				objectTags := map[string]string(nil)
				taggingDirective := strings.ToUpper(strings.TrimSpace(req.Header.Get("X-Amz-Tagging-Directive")))
				if taggingDirective == "REPLACE" {
					objectTags, err = objects.ParseTags(req.Header.Get("X-Amz-Tagging"))
					if err != nil {
						writeError(w, req, http.StatusBadRequest, "InvalidTag", "The tagging header is invalid.")
						return
					}
				}
				item, err = deps.Objects.Copy(req.Context(), sourceBucket.ID, sourceKey, sourceVersionID, bucket.ID, objects.PutInput{
					Key:                key,
					ContentType:        replaceOrEmpty(req.Header.Get("X-Amz-Metadata-Directive"), req.Header.Get("Content-Type")),
					CacheControl:       replaceOrEmpty(req.Header.Get("X-Amz-Metadata-Directive"), req.Header.Get("Cache-Control")),
					ContentDisposition: replaceOrEmpty(req.Header.Get("X-Amz-Metadata-Directive"), req.Header.Get("Content-Disposition")),
					ContentEncoding:    replaceOrEmpty(req.Header.Get("X-Amz-Metadata-Directive"), req.Header.Get("Content-Encoding")),
					UserMetadata:       userMetadata,
					Tags:               objectTags,
					CreatedBy:          createdBy,
				})
			} else {
				objectTags, err := objects.ParseTags(req.Header.Get("X-Amz-Tagging"))
				if err != nil {
					writeError(w, req, http.StatusBadRequest, "InvalidTag", "The tagging header is invalid.")
					return
				}
				item, err = deps.Objects.Put(req.Context(), bucket.ID, objects.PutInput{
					Key:                key,
					ContentType:        req.Header.Get("Content-Type"),
					CacheControl:       req.Header.Get("Cache-Control"),
					ContentDisposition: req.Header.Get("Content-Disposition"),
					ContentEncoding:    req.Header.Get("Content-Encoding"),
					UserMetadata:       extractUserMetadata(req.Header),
					Tags:               objectTags,
					CreatedBy:          createdBy,
					ExpectedSize:       expectedContentLength(req),
					ContentMD5:         req.Header.Get("Content-MD5"),
					Body:               req.Body,
				})
			}
			if err != nil {
				if strings.Contains(err.Error(), "http: request body too large") {
					writeError(w, req, http.StatusRequestEntityTooLarge, "EntityTooLarge", "The proposed upload exceeds the configured upload size limit.")
					return
				}
				switch {
				case errors.Is(err, objects.ErrIncompleteBody):
					writeError(w, req, http.StatusBadRequest, "IncompleteBody", "You did not provide the number of bytes specified by the Content-Length HTTP header.")
				case errors.Is(err, objects.ErrInvalidDigest):
					writeError(w, req, http.StatusBadRequest, "InvalidDigest", "The Content-MD5 you specified was invalid.")
				case errors.Is(err, objects.ErrBadDigest):
					writeError(w, req, http.StatusBadRequest, "BadDigest", "The Content-MD5 you specified did not match what we received.")
				case quotas.IsQuotaExceeded(err):
					writeError(w, req, http.StatusConflict, "QuotaExceeded", "The requested write exceeds the configured quota.")
				default:
					writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
				}
				return
			}
			deps.Metrics.UploadCount.Inc()
			deps.Metrics.BytesTransferred.WithLabelValues("ingress").Add(float64(item.SizeBytes))
			actorID := "presigned"
			if actor["userID"] != "" {
				actorID = actor["userID"]
			}
			_ = deps.Audit.Record(req.Context(), audit.Entry{
				Actor:     actorID,
				Action:    copyAction(copySource),
				Resource:  fmt.Sprintf("%s/%s", bucket.Name, key),
				Outcome:   "success",
				RequestID: req.Header.Get("X-Request-Id"),
				Detail:    map[string]any{"bytes": item.SizeBytes, "versionId": item.VersionID, "tagCount": len(item.Tags)},
			})
			_ = deps.Events.Emit(req.Context(), copyEvent(copySource), map[string]any{"bucketId": bucket.ID, "bucketName": bucket.Name, "objectId": item.ID, "key": key, "versionId": item.VersionID})
			applyQuotaWarningHeaders(w, req.Context(), deps.Quotas, bucket.ID, createdBy)
			w.Header().Set("ETag", quoteETag(item.ETag))
			w.Header().Set("x-amz-version-id", item.VersionID)
			w.Header().Set("Last-Modified", item.CreatedAt)
			if strings.TrimSpace(copySource) != "" {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_ = xml.NewEncoder(w).Encode(copyObjectResult{
					LastModified: item.CreatedAt,
					ETag:         quoteETag(item.ETag),
				})
				return
			}
			w.WriteHeader(http.StatusOK)
		})

		r.Get("/{bucket}/*", func(w http.ResponseWriter, req *http.Request) {
			bucketName := chi.URLParam(req, "bucket")
			key := chi.URLParam(req, "*")
			if _, ok := req.URL.Query()["tagging"]; ok {
				bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
				if err != nil {
					writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
					return
				}
				if _, status := authorizeBucketPolicyAware(req, deps, bucket.ID, key, "object.get", objectResource(bucketName, key), "s3:GetObjectTagging", bucketPolicyResource(bucketName, key)); status != authAllowed {
					writeS3AuthorizationError(w, req, status)
					return
				}
				tags, err := deps.Objects.GetTags(req.Context(), bucket.ID, key, req.URL.Query().Get("versionId"))
				if err != nil {
					writeError(w, req, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				_ = xml.NewEncoder(w).Encode(taggingResponse{TagSet: toTagPairs(tags)})
				return
			}
			bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
			if err != nil {
				writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
				return
			}
			if _, status := authorizeBucketPolicyAware(req, deps, bucket.ID, key, "object.get", objectResource(bucketName, key), "s3:GetObject", bucketPolicyResource(bucketName, key)); status != authAllowed {
				writeS3AuthorizationError(w, req, status)
				return
			}
			versionID := req.URL.Query().Get("versionId")
			var item objects.Metadata
			var reader io.ReadCloser
			if versionID == "" {
				item, reader, err = deps.Objects.GetByKey(req.Context(), bucket.ID, key)
			} else {
				item, reader, err = deps.Objects.GetByVersion(req.Context(), bucket.ID, key, versionID)
			}
			if err != nil {
				writeError(w, req, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
				return
			}
			defer reader.Close()
			applyObjectHeaders(w, item)
			deps.Metrics.DownloadCount.Inc()
			deps.Metrics.BytesTransferred.WithLabelValues("egress").Add(float64(item.SizeBytes))
			_, _ = io.Copy(w, reader)
		})

		r.Head("/{bucket}/*", func(w http.ResponseWriter, req *http.Request) {
			bucketName := chi.URLParam(req, "bucket")
			key := chi.URLParam(req, "*")
			bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
			if err != nil {
				writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
				return
			}
			if _, status := authorizeBucketPolicyAware(req, deps, bucket.ID, key, "object.get", objectResource(bucketName, key), "s3:GetObject", bucketPolicyResource(bucketName, key)); status != authAllowed {
				writeS3AuthorizationError(w, req, status)
				return
			}
			versionID := req.URL.Query().Get("versionId")
			var item objects.Metadata
			if versionID == "" {
				item, err = deps.Objects.Head(req.Context(), bucket.ID, key)
			} else {
				var reader io.ReadCloser
				item, reader, err = deps.Objects.GetByVersion(req.Context(), bucket.ID, key, versionID)
				if reader != nil {
					_ = reader.Close()
				}
			}
			if err != nil {
				writeError(w, req, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
				return
			}
			applyObjectHeaders(w, item)
			w.WriteHeader(http.StatusOK)
		})

		r.Delete("/{bucket}/*", func(w http.ResponseWriter, req *http.Request) {
			bucketName := chi.URLParam(req, "bucket")
			key := chi.URLParam(req, "*")
			if uploadID := req.URL.Query().Get("uploadId"); uploadID != "" {
				bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
				if err != nil {
					writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
					return
				}
				if _, status := authorizeBucketPolicyAware(req, deps, bucket.ID, key, "object.delete", objectResource(bucketName, key), "s3:AbortMultipartUpload", bucketPolicyResource(bucketName, key)); status != authAllowed {
					writeS3AuthorizationError(w, req, status)
					return
				}
				if err := deps.Multipart.Abort(req.Context(), uploadID); err != nil {
					writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
					return
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
			if err != nil {
				writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
				return
			}
			actor, status := authorizeBucketPolicyAware(req, deps, bucket.ID, key, "object.delete", objectResource(bucketName, key), "s3:DeleteObject", bucketPolicyResource(bucketName, key))
			if status != authAllowed {
				writeS3AuthorizationError(w, req, status)
				return
			}
			if versionID := req.URL.Query().Get("versionId"); versionID != "" {
				if err := deps.Objects.DeleteVersion(req.Context(), bucket.ID, key, versionID); err != nil {
					switch {
					case errors.Is(err, objects.ErrVersionNotFound):
						writeError(w, req, http.StatusNotFound, "NoSuchVersion", "The specified version does not exist.")
					case errors.Is(err, objects.ErrRetentionActive), errors.Is(err, objects.ErrLegalHoldActive):
						writeError(w, req, http.StatusConflict, "AccessDenied", "Object retention or legal hold is still active for this version.")
					default:
						writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
					}
					return
				}
				_ = deps.Audit.Record(req.Context(), audit.Entry{
					Actor:     actor["userID"],
					Action:    "object.version.delete",
					Resource:  fmt.Sprintf("%s/%s", bucket.Name, key),
					Outcome:   "success",
					RequestID: req.Header.Get("X-Request-Id"),
					Detail:    map[string]any{"versionId": versionID},
				})
				w.WriteHeader(http.StatusNoContent)
				return
			}
			if err := deps.Objects.Delete(req.Context(), bucket.ID, key); err != nil {
				switch {
				case errors.Is(err, objects.ErrRetentionActive), errors.Is(err, objects.ErrLegalHoldActive):
					writeError(w, req, http.StatusConflict, "AccessDenied", "Object retention or legal hold is still active for this key.")
				default:
					writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
				}
				return
			}
			_ = deps.Audit.Record(req.Context(), audit.Entry{
				Actor:     actor["userID"],
				Action:    "object.delete",
				Resource:  fmt.Sprintf("%s/%s", bucket.Name, key),
				Outcome:   "success",
				RequestID: req.Header.Get("X-Request-Id"),
				Detail:    map[string]any{},
			})
			_ = deps.Events.Emit(req.Context(), "object.deleted", map[string]any{"bucketId": bucket.ID, "bucketName": bucket.Name, "key": key})
			w.WriteHeader(http.StatusNoContent)
		})

		r.Post("/presign", func(w http.ResponseWriter, req *http.Request) {
			accessKey := req.Header.Get("X-S3P-Access-Key")
			secretKey := req.Header.Get("X-S3P-Secret")
			if _, err := deps.Credentials.Validate(req.Context(), accessKey, secretKey); err != nil {
				writeError(w, req, http.StatusUnauthorized, "AccessDenied", "The request is not authorized.")
				return
			}
			cred, err := deps.Credentials.Lookup(req.Context(), accessKey)
			if err != nil {
				writeError(w, req, http.StatusUnauthorized, "InvalidAccessKeyId", "The AWS access key Id you provided does not exist in our records.")
				return
			}
			var body struct {
				Method string `json:"method"`
				Path   string `json:"path"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				writeError(w, req, http.StatusBadRequest, "InvalidRequest", "The request payload is invalid.")
				return
			}
			action := "object.get"
			resource := "*"
			if strings.EqualFold(body.Method, http.MethodPut) {
				action = "object.put"
			}
			if strings.HasPrefix(body.Path, "/s3/") {
				parts := strings.SplitN(strings.TrimPrefix(body.Path, "/s3/"), "/", 2)
				if len(parts) == 2 {
					resource = objectResource(parts[0], parts[1])
				}
			}
			if err := deps.Authorizer.CheckRole(req.Context(), cred["role"], action, resource); err != nil {
				writeError(w, req, http.StatusForbidden, "AccessDenied", "The request is not authorized.")
				return
			}
			urlValue, err := GeneratePresignedURL(body.Method, "http://"+req.Host+body.Path, accessKey, cred["secretKey"], deps.Region, deps.PresignTTL, time.Now().UTC())
			if err != nil {
				writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"url": urlValue,
			})
		})

		r.Post("/{bucket}/*", func(w http.ResponseWriter, req *http.Request) {
			bucketName := chi.URLParam(req, "bucket")
			key := chi.URLParam(req, "*")
			bucket, err := deps.Buckets.GetByName(req.Context(), bucketName)
			if err != nil {
				writeError(w, req, http.StatusNotFound, "NoSuchBucket", "The specified bucket does not exist.")
				return
			}
			actor, status := authorizeBucketPolicyAware(req, deps, bucket.ID, key, "object.put", objectResource(bucketName, key), "s3:PutObject", bucketPolicyResource(bucketName, key))
			if status != authAllowed {
				writeS3AuthorizationError(w, req, status)
				return
			}
			if _, ok := req.URL.Query()["uploads"]; ok {
				upload, err := deps.Multipart.Initiate(req.Context(), bucket.ID, multipart.InitiateInput{
					ObjectKey:          key,
					ContentType:        req.Header.Get("Content-Type"),
					CacheControl:       req.Header.Get("Cache-Control"),
					ContentDisposition: req.Header.Get("Content-Disposition"),
					ContentEncoding:    req.Header.Get("Content-Encoding"),
					UserMetadata:       extractUserMetadata(req.Header),
					InitiatedBy:        actor["userID"],
				})
				if err != nil {
					writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				_ = xml.NewEncoder(w).Encode(initiateMultipartUploadResult{
					Bucket:   bucket.Name,
					Key:      key,
					UploadID: upload.ID,
				})
				return
			}
			if uploadID := req.URL.Query().Get("uploadId"); uploadID != "" {
				orderedParts, err := multipart.ParseCompleteRequest(req.Body)
				if err != nil || len(orderedParts) == 0 {
					writeError(w, req, http.StatusBadRequest, "InvalidRequest", "The multipart completion payload is invalid.")
					return
				}
				item, err := deps.Multipart.Complete(req.Context(), uploadID, actor["userID"], orderedParts)
				if err != nil {
					switch {
					case errors.Is(err, multipart.ErrInvalidPart):
						writeError(w, req, http.StatusBadRequest, "InvalidPart", "One or more of the specified parts could not be found.")
					case errors.Is(err, multipart.ErrInvalidPartOrder):
						writeError(w, req, http.StatusBadRequest, "InvalidPartOrder", "The list of parts was not in ascending order.")
					case quotas.IsQuotaExceeded(err):
						writeError(w, req, http.StatusConflict, "QuotaExceeded", "The requested write exceeds the configured quota.")
					default:
						writeError(w, req, http.StatusInternalServerError, "InternalError", "An internal error occurred.")
					}
					return
				}
				_ = deps.Events.Emit(req.Context(), "multipart.completed", map[string]any{"bucketId": bucket.ID, "bucketName": bucket.Name, "objectId": item.ID, "key": key, "uploadId": uploadID})
				applyQuotaWarningHeaders(w, req.Context(), deps.Quotas, bucket.ID, actor["userID"])
				w.Header().Set("x-amz-version-id", item.VersionID)
				w.Header().Set("Content-Type", "application/xml")
				_ = xml.NewEncoder(w).Encode(completeMultipartUploadResult{
					Bucket: bucket.Name,
					Key:    key,
					ETag:   quoteETag(item.ETag),
				})
				return
			}
			writeError(w, req, http.StatusBadRequest, "InvalidRequest", "The requested POST operation is not supported.")
		})
	})
}

func buildListObjectsV2Response(bucketName, prefix string, query url.Values, items []objects.Metadata) listObjectsV2Response {
	maxKeys := 1000
	if raw := strings.TrimSpace(query.Get("max-keys")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			maxKeys = parsed
		}
	}

	startKey := strings.TrimSpace(query.Get("continuation-token"))
	if startKey == "" {
		startKey = strings.TrimSpace(query.Get("start-after"))
	}

	filtered := items
	if startKey != "" {
		idx := 0
		for idx < len(filtered) && filtered[idx].Key <= startKey {
			idx++
		}
		filtered = filtered[idx:]
	}

	resp := listObjectsV2Response{
		Name:              bucketName,
		Prefix:            prefix,
		MaxKeys:           maxKeys,
		ContinuationToken: strings.TrimSpace(query.Get("continuation-token")),
		StartAfter:        strings.TrimSpace(query.Get("start-after")),
	}

	limit := len(filtered)
	if maxKeys < limit {
		limit = maxKeys
		resp.IsTruncated = true
		if limit > 0 {
			resp.NextContinuationToken = filtered[limit-1].Key
		}
	}
	for _, item := range filtered[:limit] {
		resp.Contents = append(resp.Contents, struct {
			Key          string `xml:"Key"`
			Size         int64  `xml:"Size"`
			LastModified string `xml:"LastModified"`
			ETag         string `xml:"ETag"`
		}{
			Key:          item.Key,
			Size:         item.SizeBytes,
			LastModified: item.CreatedAt,
			ETag:         quoteETag(item.ETag),
		})
	}
	resp.KeyCount = len(resp.Contents)
	return resp
}

type errorResponse struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource"`
	RequestID string   `xml:"RequestId"`
}

func writeError(w http.ResponseWriter, req *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_ = xml.NewEncoder(w).Encode(errorResponse{
		Code:      code,
		Message:   message,
		Resource:  req.URL.Path,
		RequestID: req.Header.Get("X-Request-Id"),
	})
}

func isPresigned(req *http.Request) bool {
	return req.URL.Query().Get("accessKey") != ""
}

func validatePresigned(req *http.Request, creds *credentials.Service) (map[string]string, bool) {
	accessKey := req.URL.Query().Get("accessKey")
	expires, err := strconv.ParseInt(req.URL.Query().Get("expires"), 10, 64)
	if err != nil || time.Now().UTC().Unix() > expires {
		return nil, false
	}
	signature := req.URL.Query().Get("signature")
	cred, err := creds.Lookup(req.Context(), accessKey)
	if err != nil {
		return nil, false
	}
	expected := sign(accessKey, cred["secretHash"], req.Method, req.URL.Path, expires)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return nil, false
	}
	return map[string]string{"userID": cred["userID"], "role": cred["role"], "accessKey": accessKey}, true
}

func sign(accessKey, keyMaterial, method, path string, expires int64) string {
	mac := hmac.New(sha256.New, []byte(keyMaterial))
	_, _ = mac.Write([]byte(fmt.Sprintf("%s:%s:%s:%d", accessKey, method, path, expires)))
	return hex.EncodeToString(mac.Sum(nil))
}

func expectedContentLength(req *http.Request) int64 {
	if req.ContentLength < 0 {
		return -1
	}
	return req.ContentLength
}

func extractUserMetadata(headers http.Header) map[string]string {
	metadata := map[string]string{}
	for key, values := range headers {
		lowerKey := strings.ToLower(key)
		if !strings.HasPrefix(lowerKey, "x-amz-meta-") || len(values) == 0 {
			continue
		}
		name := strings.TrimPrefix(lowerKey, "x-amz-meta-")
		metadata[name] = values[0]
	}
	return metadata
}

func applyObjectHeaders(w http.ResponseWriter, item objects.Metadata) {
	w.Header().Set("Content-Type", item.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(item.SizeBytes, 10))
	w.Header().Set("ETag", quoteETag(item.ETag))
	w.Header().Set("x-amz-version-id", item.VersionID)
	w.Header().Set("Last-Modified", item.CreatedAt)
	if item.CacheControl != "" {
		w.Header().Set("Cache-Control", item.CacheControl)
	}
	if item.ContentDisposition != "" {
		w.Header().Set("Content-Disposition", item.ContentDisposition)
	}
	if item.ContentEncoding != "" {
		w.Header().Set("Content-Encoding", item.ContentEncoding)
	}
	keys := make([]string, 0, len(item.UserMetadata))
	for key := range item.UserMetadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		w.Header().Set("x-amz-meta-"+key, item.UserMetadata[key])
	}
	if len(item.Tags) > 0 {
		w.Header().Set("x-amz-tagging-count", strconv.Itoa(len(item.Tags)))
	}
}

func quoteETag(etag string) string {
	return `"` + etag + `"`
}

type authStatus int

const (
	authAllowed authStatus = iota
	authUnauthenticated
	authForbidden
	authBadSignature
)

func authorizeS3Request(req *http.Request, deps RouterDeps, action, resource string) (map[string]string, authStatus) {
	actor, status := authenticateS3Identity(req, deps)
	if status != authAllowed {
		return nil, status
	}
	return authorizeRole(req, deps, actor["role"], actor["userID"], actor["accessKey"], action, resource)
}

func authenticateS3Identity(req *http.Request, deps RouterDeps) (map[string]string, authStatus) {
	if isSigV4Presigned(req) {
		identity, err := validatePresignedSigV4(req, deps.Credentials, deps.Region, time.Now().UTC())
		if err != nil {
			return nil, authBadSignature
		}
		return map[string]string{"role": identity.Role, "userID": identity.UserID, "accessKey": identity.AccessKey}, authAllowed
	}
	if isPresigned(req) {
		actor, ok := validatePresigned(req, deps.Credentials)
		if !ok {
			return nil, authBadSignature
		}
		return actor, authAllowed
	}
	actor, ok := authorize(req, deps.Credentials, deps.Region)
	if !ok {
		return nil, authUnauthenticated
	}
	return actor, authAllowed
}

func authorizeBucketPolicyAware(req *http.Request, deps RouterDeps, bucketID, key, action, resource, awsAction, awsResource string) (map[string]string, authStatus) {
	actor, authnStatus := authenticateS3Identity(req, deps)
	if authnStatus == authBadSignature {
		return nil, authBadSignature
	}

	decision := policies.PolicyDecisionNone
	if deps.Policies != nil && bucketID != "" && awsAction != "" && awsResource != "" {
		principal := "*"
		if actor != nil && actor["accessKey"] != "" {
			principal = actor["accessKey"]
		}
		policyDecision, err := deps.Policies.EvaluateBucketPolicy(req.Context(), bucketID, principal, awsAction, awsResource, bucketPolicyRequestContext(req))
		if err != nil {
			return nil, authUnauthenticated
		}
		decision = policyDecision
	}

	if decision == policies.PolicyDecisionDeny {
		return nil, authForbidden
	}
	if authnStatus == authAllowed {
		authorized, status := authorizeRole(req, deps, actor["role"], actor["userID"], actor["accessKey"], action, resource)
		if status == authAllowed {
			return authorized, authAllowed
		}
		if decision == policies.PolicyDecisionAllow {
			return actor, authAllowed
		}
		return nil, status
	}
	if decision == policies.PolicyDecisionAllow {
		return map[string]string{"role": "public", "accessKey": "*"}, authAllowed
	}
	return nil, authUnauthenticated
}

func authorizeRole(req *http.Request, deps RouterDeps, role, userID, accessKey, action, resource string) (map[string]string, authStatus) {
	resolvedRole, err := deps.Authorizer.CheckSubject(req.Context(), "credential", accessKey, role, action, resource)
	if err != nil {
		if errors.Is(err, authz.ErrForbidden) {
			return nil, authForbidden
		}
		return nil, authUnauthenticated
	}
	return map[string]string{"role": resolvedRole, "userID": userID, "accessKey": accessKey}, authAllowed
}

func writeS3AuthorizationError(w http.ResponseWriter, req *http.Request, status authStatus) {
	switch status {
	case authBadSignature:
		writeError(w, req, http.StatusUnauthorized, "SignatureDoesNotMatch", "The request signature we calculated does not match.")
	case authForbidden:
		writeError(w, req, http.StatusForbidden, "AccessDenied", "The request is not authorized.")
	default:
		writeError(w, req, http.StatusUnauthorized, "AccessDenied", "The request is not authorized.")
	}
}

func bucketResource(bucket string) string {
	return "bucket:" + bucket
}

func objectResource(bucket, key string) string {
	return "bucket:" + bucket + "/object:" + key
}

func bucketPolicyResource(bucket, key string) string {
	if key == "" {
		return "arn:aws:s3:::" + bucket
	}
	return "arn:aws:s3:::" + bucket + "/" + key
}

func bucketPolicyRequestContext(req *http.Request) map[string]string {
	context := map[string]string{}
	if prefix := strings.TrimSpace(req.URL.Query().Get("prefix")); prefix != "" {
		context["s3:prefix"] = prefix
	}
	if ip := s3RequestIP(req); ip != "" {
		context["aws:SourceIp"] = ip
	}
	return context
}

func s3RequestIP(req *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(req.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	return strings.TrimSpace(req.RemoteAddr)
}

func applyQuotaWarningHeaders(w http.ResponseWriter, ctx context.Context, quotaSvc *quotas.Service, bucketID, userID string) {
	if quotaSvc == nil {
		return
	}
	warnings, err := quotaSvc.CurrentWarnings(ctx, bucketID, userID)
	if err != nil {
		return
	}
	if warnings.BucketBytes {
		w.Header().Set("X-S3P-Quota-Warning-Bucket-Bytes", "true")
	}
	if warnings.BucketCount {
		w.Header().Set("X-S3P-Quota-Warning-Bucket-Objects", "true")
	}
	if warnings.UserBytes {
		w.Header().Set("X-S3P-Quota-Warning-User-Bytes", "true")
	}
}

func parseCopySource(raw string) (bucket, key, versionID string, err error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" {
		return "", "", "", errors.New("empty copy source")
	}
	if strings.Contains(trimmed, "?") {
		parts := strings.SplitN(trimmed, "?", 2)
		trimmed = parts[0]
		values, parseErr := url.ParseQuery(parts[1])
		if parseErr != nil {
			return "", "", "", parseErr
		}
		versionID = values.Get("versionId")
	}
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", "", errors.New("invalid copy source")
	}
	key, err = url.PathUnescape(parts[1])
	if err != nil {
		return "", "", "", err
	}
	return parts[0], key, versionID, nil
}

func parseTaggingXML(body io.Reader) (map[string]string, error) {
	var payload taggingResponse
	if err := xml.NewDecoder(body).Decode(&payload); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(payload.TagSet))
	for _, tag := range payload.TagSet {
		if tag.Key == "" {
			continue
		}
		result[tag.Key] = tag.Value
	}
	return objects.NormalizeTags(result), nil
}

func toTagPairs(tags map[string]string) []tagPair {
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]tagPair, 0, len(keys))
	for _, key := range keys {
		result = append(result, tagPair{Key: key, Value: tags[key]})
	}
	return result
}

func replaceOrEmpty(directive, value string) string {
	if strings.EqualFold(strings.TrimSpace(directive), "REPLACE") {
		return value
	}
	return ""
}

func copyAction(copySource string) string {
	if strings.TrimSpace(copySource) != "" {
		return "object.copy"
	}
	return "object.put"
}

func copyEvent(copySource string) string {
	if strings.TrimSpace(copySource) != "" {
		return "object.copied"
	}
	return "object.created"
}
