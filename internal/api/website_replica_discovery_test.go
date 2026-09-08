package api

import "testing"

func TestReplicaDiscoveryRejectsUnboundOrUnsafePresentation(t *testing.T) {
	baseline := WebsiteReplicaDiscovery{ReplicaID: "11111111-1111-4111-8111-111111111111", ShortCode: "VMR-ABCDEFGHIJKLMNOPQRST", Title: "Website", PreviewURL: "https://example.com/site", ViceMeWorkURL: "https://viceme.cn/alice/site", DiscoveryURL: "https://viceme.cn/alice/site/discover", Creator: WebsiteReplicaCreator{Handle: "alice"}}
	if !validWebsiteReplicaDiscovery(baseline, baseline.ShortCode) {
		t.Fatal("valid empty discovery rejected")
	}
	for _, change := range []func(*WebsiteReplicaDiscovery){
		func(v *WebsiteReplicaDiscovery) { v.ShortCode = "other" },
		func(v *WebsiteReplicaDiscovery) { v.PreviewURL = "javascript:alert(1)" },
		func(v *WebsiteReplicaDiscovery) { v.Statistics.AcquisitionCount = -1 },
		func(v *WebsiteReplicaDiscovery) { v.Statistics.CommentCount = -1 },
	} {
		value := baseline
		change(&value)
		if validWebsiteReplicaDiscovery(value, baseline.ShortCode) {
			t.Fatalf("invalid discovery accepted: %#v", value)
		}
	}
}
