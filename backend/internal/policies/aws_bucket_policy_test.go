package policies

import "testing"

func TestNormalizeBucketPolicySupportsAWSStyleDocument(t *testing.T) {
	raw := []byte(`{
  "Version":"2012-10-17",
  "Statement":[
    {
      "Sid":"PublicRead",
      "Effect":"Allow",
      "Principal":"*",
      "Action":["s3:GetObject","s3:GetObjectTagging"],
      "Resource":"arn:aws:s3:::demo-bucket/*"
    }
  ]
}`)

	document, canonical, err := normalizeBucketPolicy(raw)
	if err != nil {
		t.Fatalf("normalize bucket policy: %v", err)
	}
	if document.Version != "2012-10-17" {
		t.Fatalf("unexpected version %s", document.Version)
	}
	if len(document.Statement) != 1 || document.Statement[0].Effect != "Allow" {
		t.Fatalf("unexpected statements %#v", document.Statement)
	}
	if len(canonical) == 0 {
		t.Fatal("expected canonical json output")
	}
}

func TestEvaluateBucketPolicyHonorsExplicitDeny(t *testing.T) {
	document := normalizedBucketPolicyDocument{
		Version: "2012-10-17",
		Statement: []normalizedBucketPolicyStatement{
			{
				Effect:     "Allow",
				Principals: []string{"*"},
				Actions:    []string{"s3:GetObject"},
				Resources:  []string{"arn:aws:s3:::demo-bucket/*"},
			},
			{
				Effect:     "Deny",
				Principals: []string{"AKIA-LOCKED"},
				Actions:    []string{"s3:GetObject"},
				Resources:  []string{"arn:aws:s3:::demo-bucket/private/*"},
			},
		},
	}

	if decision := evaluateBucketPolicy(document, "*", "s3:GetObject", "arn:aws:s3:::demo-bucket/public.txt", nil); decision != PolicyDecisionAllow {
		t.Fatalf("expected allow, got %s", decision)
	}
	if decision := evaluateBucketPolicy(document, "AKIA-LOCKED", "s3:GetObject", "arn:aws:s3:::demo-bucket/private/secret.txt", nil); decision != PolicyDecisionDeny {
		t.Fatalf("expected deny, got %s", decision)
	}
}

func TestNormalizePolicyPrincipalSupportsIAMArn(t *testing.T) {
	got := normalizePolicyPrincipal("arn:aws:iam::123456789012:user/AKIAEXAMPLE")
	if got != "AKIAEXAMPLE" {
		t.Fatalf("expected access key style principal, got %s", got)
	}
}

func TestEvaluateBucketPolicyConditions(t *testing.T) {
	document := normalizedBucketPolicyDocument{
		Version: "2012-10-17",
		Statement: []normalizedBucketPolicyStatement{
			{
				Effect:     "Allow",
				Principals: []string{"*"},
				Actions:    []string{"s3:ListBucket"},
				Resources:  []string{"arn:aws:s3:::demo-bucket"},
				Conditions: []bucketPolicyCondition{
					{Operator: "StringEquals", Key: "s3:prefix", Values: []string{"public/"}},
					{Operator: "IpAddress", Key: "aws:SourceIp", Values: []string{"127.0.0.1/32"}},
				},
			},
		},
	}

	allowed := evaluateBucketPolicy(document, "*", "s3:ListBucket", "arn:aws:s3:::demo-bucket", map[string]string{
		"s3:prefix":    "public/",
		"aws:SourceIp": "127.0.0.1",
	})
	if allowed != PolicyDecisionAllow {
		t.Fatalf("expected allow, got %s", allowed)
	}

	denied := evaluateBucketPolicy(document, "*", "s3:ListBucket", "arn:aws:s3:::demo-bucket", map[string]string{
		"s3:prefix":    "private/",
		"aws:SourceIp": "127.0.0.1",
	})
	if denied != PolicyDecisionNone {
		t.Fatalf("expected none, got %s", denied)
	}
}

func TestNormalizeBucketPolicySupportsNotPrincipal(t *testing.T) {
	raw := []byte(`{
  "Version":"2012-10-17",
  "Statement":[
    {
      "Sid":"DenyOthers",
      "Effect":"Deny",
      "NotPrincipal":{"AWS":"AKIA-ALLOWED"},
      "Action":"s3:GetObject",
      "Resource":"arn:aws:s3:::demo-bucket/private/*"
    }
  ]
}`)

	document, _, err := normalizeBucketPolicy(raw)
	if err != nil {
		t.Fatalf("normalize bucket policy with NotPrincipal: %v", err)
	}
	if len(document.Statement) != 1 {
		t.Fatalf("unexpected statements %#v", document.Statement)
	}
	if got := document.Statement[0].NotPrincipals; len(got) != 1 || got[0] != "AKIA-ALLOWED" {
		t.Fatalf("unexpected not principals %#v", got)
	}
}

func TestEvaluateBucketPolicyHonorsNotPrincipal(t *testing.T) {
	document := normalizedBucketPolicyDocument{
		Version: "2012-10-17",
		Statement: []normalizedBucketPolicyStatement{
			{
				Effect:        "Deny",
				NotPrincipals: []string{"AKIA-ALLOWED"},
				Actions:       []string{"s3:GetObject"},
				Resources:     []string{"arn:aws:s3:::demo-bucket/private/*"},
			},
		},
	}

	if decision := evaluateBucketPolicy(document, "AKIA-ALLOWED", "s3:GetObject", "arn:aws:s3:::demo-bucket/private/secret.txt", nil); decision != PolicyDecisionNone {
		t.Fatalf("expected none for excluded principal, got %s", decision)
	}
	if decision := evaluateBucketPolicy(document, "AKIA-OTHER", "s3:GetObject", "arn:aws:s3:::demo-bucket/private/secret.txt", nil); decision != PolicyDecisionDeny {
		t.Fatalf("expected deny for non-excluded principal, got %s", decision)
	}
}

func TestEvaluateBucketPolicyNegativeStringConditions(t *testing.T) {
	document := normalizedBucketPolicyDocument{
		Version: "2012-10-17",
		Statement: []normalizedBucketPolicyStatement{
			{
				Effect:     "Allow",
				Principals: []string{"*"},
				Actions:    []string{"s3:ListBucket"},
				Resources:  []string{"arn:aws:s3:::demo-bucket"},
				Conditions: []bucketPolicyCondition{
					{Operator: "StringNotEquals", Key: "s3:prefix", Values: []string{"private/"}},
					{Operator: "StringNotLike", Key: "s3:prefix", Values: []string{"secret*"}},
				},
			},
		},
	}

	allowed := evaluateBucketPolicy(document, "*", "s3:ListBucket", "arn:aws:s3:::demo-bucket", map[string]string{
		"s3:prefix": "public/",
	})
	if allowed != PolicyDecisionAllow {
		t.Fatalf("expected allow, got %s", allowed)
	}

	denied := evaluateBucketPolicy(document, "*", "s3:ListBucket", "arn:aws:s3:::demo-bucket", map[string]string{
		"s3:prefix": "private/",
	})
	if denied != PolicyDecisionNone {
		t.Fatalf("expected none for StringNotEquals mismatch, got %s", denied)
	}

	denied = evaluateBucketPolicy(document, "*", "s3:ListBucket", "arn:aws:s3:::demo-bucket", map[string]string{
		"s3:prefix": "secret-folder/",
	})
	if denied != PolicyDecisionNone {
		t.Fatalf("expected none for StringNotLike mismatch, got %s", denied)
	}
}

func TestEvaluateBucketPolicyNotIPAddressCondition(t *testing.T) {
	document := normalizedBucketPolicyDocument{
		Version: "2012-10-17",
		Statement: []normalizedBucketPolicyStatement{
			{
				Effect:     "Deny",
				Principals: []string{"*"},
				Actions:    []string{"s3:GetObject"},
				Resources:  []string{"arn:aws:s3:::demo-bucket/private/*"},
				Conditions: []bucketPolicyCondition{
					{Operator: "NotIpAddress", Key: "aws:SourceIp", Values: []string{"127.0.0.1/32"}},
				},
			},
		},
	}

	allowed := evaluateBucketPolicy(document, "*", "s3:GetObject", "arn:aws:s3:::demo-bucket/private/secret.txt", map[string]string{
		"aws:SourceIp": "127.0.0.1",
	})
	if allowed != PolicyDecisionNone {
		t.Fatalf("expected none for allowed source IP, got %s", allowed)
	}

	denied := evaluateBucketPolicy(document, "*", "s3:GetObject", "arn:aws:s3:::demo-bucket/private/secret.txt", map[string]string{
		"aws:SourceIp": "10.0.0.25",
	})
	if denied != PolicyDecisionDeny {
		t.Fatalf("expected deny for non-matching source IP, got %s", denied)
	}
}

func TestNormalizeBucketPolicySupportsPrincipalList(t *testing.T) {
	raw := []byte(`{
  "Version":"2012-10-17",
  "Statement":[
    {
      "Sid":"AllowSpecificPrincipals",
      "Effect":"Allow",
      "Principal":{"AWS":["AKIA-ONE","arn:aws:iam::123456789012:user/AKIA-TWO"]},
      "Action":"s3:GetObject",
      "Resource":"arn:aws:s3:::demo-bucket/*"
    }
  ]
}`)

	document, _, err := normalizeBucketPolicy(raw)
	if err != nil {
		t.Fatalf("normalize bucket policy with principal list: %v", err)
	}
	got := document.Statement[0].Principals
	if len(got) != 2 || got[0] != "AKIA-ONE" || got[1] != "AKIA-TWO" {
		t.Fatalf("unexpected principals %#v", got)
	}
}
