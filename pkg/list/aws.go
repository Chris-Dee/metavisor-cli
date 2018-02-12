package list

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

const (
	prodBucketName   = "solo-brkt-prod-net"
	prodBucketRegion = "us-west-2"
	mvPrefix         = "metavisor"

	latestKey = "latest/amis.json"
	keySuffix = "/amis.json"
)

type mvVersions []string

func (v mvVersions) Len() int      { return len(v) }
func (v mvVersions) Swap(i, j int) { v[i], v[j] = v[j], v[i] }

type byVersion struct{ mvVersions }

func (v byVersion) Less(i, j int) bool {
	// Verison has format e.g. metavisor-2-19-49-g617a92b81
	p1 := strings.Split(v.mvVersions[i], "-")
	p2 := strings.Split(v.mvVersions[j], "-")
	maj1, _ := strconv.Atoi(p1[1])
	min1, _ := strconv.Atoi(p1[2])
	b1, _ := strconv.Atoi(p1[3])
	maj2, _ := strconv.Atoi(p2[1])
	min2, _ := strconv.Atoi(p2[2])
	b2, _ := strconv.Atoi(p2[3])
	if maj1 < maj2 {
		return true
	}
	if maj1 > maj2 {
		return false
	}

	if min1 < min2 {
		return true
	}
	if min1 > min2 {
		return false
	}

	return b1 <= b2
}

func awsGetMVVersions() (MetavisorVersions, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(prodBucketRegion),
	})
	if err != nil {
		return MetavisorVersions{}, err
	}
	s3C := s3.New(sess)
	mvs, err := listAllMetavisors(s3C)
	if err != nil {
		return MetavisorVersions{}, err
	}
	latest, err := determineLatest(s3C, mvs)
	if err != nil {
		latest = ""
	}
	return MetavisorVersions{
		Latest:   latest,
		Versions: mvs,
	}, nil
}

func listAllMetavisors(client *s3.S3) (mvVersions, error) {
	out, err := client.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket: aws.String(prodBucketName),
		Prefix: aws.String(mvPrefix),
	})
	if err != nil {
		return nil, err
	}
	versions := map[string]struct{}{}
	for _, obj := range out.Contents {
		versions[strings.Split(*obj.Key, "/")[0]] = struct{}{}
	}
	versionSlice := mvVersions{}
	for key := range versions {
		versionSlice = append(versionSlice, key)
	}
	sort.Sort(sort.Reverse(byVersion{versionSlice}))
	return versionSlice, nil
}

func determineLatest(client *s3.S3, allVersions []string) (string, error) {
	latest, err := getObjectBody(client, latestKey)
	if err != nil {
		return "", err
	}
	for i := range allVersions {
		v, err := getObjectBody(client, fmt.Sprintf("%s%s", allVersions[i], keySuffix))
		if err != nil {
			return "", err
		}
		if reflect.DeepEqual(latest, v) {
			return allVersions[i], nil
		}
	}
	return "", errors.New("No latest version")
}

func getObjectBody(client *s3.S3, key string) (map[string]string, error) {
	latest, err := client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(prodBucketName),
		Key:    aws.String(latestKey),
	})
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(latest.Body)
	latestMap := make(map[string]string)
	err = decoder.Decode(&latestMap)
	if err != nil {
		return nil, err
	}
	return latestMap, nil
}
