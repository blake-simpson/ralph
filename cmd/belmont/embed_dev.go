//go:build !embed

package main

import "embed"

var embeddedSkills embed.FS
var embeddedAgents embed.FS
var embeddedPrompts embed.FS

var hasEmbeddedFiles = false
