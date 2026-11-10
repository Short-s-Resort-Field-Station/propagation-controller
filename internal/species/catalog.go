// Package species holds the specimen catalog for shelf assignment. Since
// 2026-11-10 it mirrors the Short's Resort Field Station website's species
// plates (fs-species.jsx, 29 woody plates) plus Inland Sea Oats from the
// site's grass library — the website is the canonical list (Austin). Each
// specimen keeps a stable 4-letter botanical ID (genus+species initials) that
// species profiles key off, its filing category, primary propagation method
// (seed | cuttings | hybrid, derived from the plate's method text), and the
// plate's difficulty (1–5). The UI's species picker draws from this list;
// free-text entry remains possible through the zone config API.
package species

type Species struct {
	ID         string `json:"id"` // botanical ID, e.g. QUMA
	Common     string `json:"common_name"`
	Latin      string `json:"latin_name"`
	Category   string `json:"category"`   // canopy | nurse | riparian | wildlife | specialty | shrub | vine | grass
	Method     string `json:"method"`     // seed | cuttings | hybrid
	Difficulty int    `json:"difficulty"` // 1 (easiest) – 5
}

// Catalog is alphabetical by common name, as the website files it.
var Catalog = []Species{
	{"MATR", "Agarita", "Mahonia trifoliolata", "shrub", "hybrid", 4},                        // website plate N
	{"TADI", "Bald Cypress", "Taxodium distichum", "riparian", "hybrid", 1},                  // website plate G
	{"ACGR", "Bigtooth Maple", "Acer grandidentatum", "specialty", "seed", 4},                // website plate L
	{"PRSE", "Black Cherry", "Prunus serotina var. eximia", "wildlife", "seed", 2},           // website plate I
	{"SANI", "Black Willow", "Salix nigra", "riparian", "cuttings", 1},                       // website plate D
	{"QUMA", "Bur Oak", "Quercus macrocarpa", "canopy", "seed", 2},                           // website plate B
	{"CEOC", "Buttonbush", "Cephalanthus occidentalis", "shrub", "hybrid", 2},                // website plate X
	{"ULCR", "Cedar Elm", "Ulmus crassifolia", "canopy", "seed", 1},                          // website plate E
	{"PRAN", "Chickasaw Plum", "Prunus angustifolia", "wildlife", "cuttings", 2},             // website plate R
	{"QUMU", "Chinkapin Oak", "Quercus muehlenbergii", "canopy", "seed", 2},                  // website plate Z
	{"PODE", "Eastern Cottonwood", "Populus deltoides", "riparian", "cuttings", 1},           // website plate U
	{"FOPU", "Elbowbush", "Forestiera pubescens", "shrub", "hybrid", 3},                      // website plate O
	{"RHLA", "Flameleaf Sumac", "Rhus lanceolata", "shrub", "hybrid", 2},                     // website plate FL
	{"RHAR", "Fragrant Sumac", "Rhus aromatica", "shrub", "hybrid", 3},                       // website plate P
	{"CHLA", "Inland Sea Oats", "Chasmanthium latifolium", "grass", "hybrid", 1},             // website plate lib
	{"QUVI", "Live Oak", "Quercus fusiformis", "canopy", "seed", 2},                          // website plate C
	{"UNSP", "Mexican Buckeye", "Ungnadia speciosa", "shrub", "seed", 2},                     // website plate MB
	{"PRME", "Mexican Plum", "Prunus mexicana", "wildlife", "hybrid", 2},                     // website plate J
	{"VIMU", "Mustang Grape", "Vitis mustangensis", "vine", "cuttings", 3},                   // website plate S
	{"CERE", "Netleaf Hackberry", "Celtis laevigata var. reticulata", "wildlife", "seed", 1}, // website plate Y
	{"CAIL", "Pecan", "Carya illinoinensis", "riparian", "seed", 1},                          // website plate F
	{"ILDE", "Possumhaw Holly", "Ilex decidua", "shrub", "cuttings", 3},                      // website plate V
	{"QUST", "Post Oak", "Quercus stellata", "canopy", "seed", 2},                            // website plate A
	{"CELA", "Sugarberry", "Celtis laevigata var. laevigata", "wildlife", "seed", 1},         // website plate H
	{"ARXA", "Texas Madrone", "Arbutus xalapensis", "specialty", "seed", 5},                  // website plate M
	{"SOSE", "Texas Mountain Laurel", "Sophora secundiflora", "shrub", "seed", 3},            // website plate ML
	{"DITE", "Texas Persimmon", "Diospyros texana", "wildlife", "hybrid", 2},                 // website plate K
	{"CECA", "Texas Redbud", "Cercis canadensis var. texensis", "wildlife", "seed", 3},       // website plate W
	{"PLOC", "Texas Sycamore", "Platanus occidentalis", "riparian", "seed", 2},               // website plate Q
	{"ILVO", "Yaupon Holly", "Ilex vomitoria", "shrub", "cuttings", 3},                       // website plate T
}
