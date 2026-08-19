package adapters

import (
	"fmt"
	"strings"

	"github.com/on-mission/landing/internal/harness"
)

func personaDelivery(persona *harness.Persona, delivery harness.PersonaDelivery) *harness.PersonaDelivery {
	if persona == nil {
		return nil
	}

	return &delivery
}

func promptWithPersona(prompt string, persona *harness.Persona) string {
	if persona == nil {
		return prompt
	}

	return fmt.Sprintf("%s\n\n%s\n\n%s", persona.Instructions, personaReferenceMaterial(*persona), prompt)
}

func personaReferenceMaterial(persona harness.Persona) string {
	files := "no files"
	if len(persona.ReferenceFiles) > 0 {
		files = strings.Join(persona.ReferenceFiles, ", ")
	}

	return fmt.Sprintf("Use the project reference material in %q as part of this perspective. The available files are %s; consult the ones that apply before doing the work.", persona.Directory, files)
}
