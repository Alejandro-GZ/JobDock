package domain

import (
	"errors"
	"regexp"
)

const ManagedImageMediaType = "application/vnd.jobdock.image.archive.v1+tar"

var managedReferencePattern = regexp.MustCompile(`^jobdock://build/([^/@]+)@(sha256:[a-f0-9]{64})$`)

func ManagedArtifactReference(buildID, digest string) string {
	return "jobdock://build/" + buildID + "@" + digest
}

func ParseManagedArtifactReference(reference string) (buildID, digest string, managed bool, err error) {
	if len(reference) < len("jobdock://") || reference[:len("jobdock://")] != "jobdock://" {
		return "", "", false, nil
	}
	matches := managedReferencePattern.FindStringSubmatch(reference)
	if len(matches) != 3 {
		return "", "", true, errors.New("invalid managed artifact reference")
	}
	return matches[1], matches[2], true, nil
}
