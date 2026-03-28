package handlers

import "cloud.google.com/go/firestore"

// firestoreMerge returns a MergeAll option for Firestore Set operations
func firestoreMerge() firestore.SetOption {
	return firestore.MergeAll
}
