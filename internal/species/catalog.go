// Package species holds the specimen catalog for shelf assignment, generated
// from the Species Register of the Native Tree & Shrub Propagation Manual
// (SR–PROP–01, §07 Species Compendium — the register that replaced the old
// A–U plate system). Each specimen carries the register's 4-letter botanical
// ID, its filing category, primary propagation method, and difficulty (1–5).
// The UI's species picker draws from this list; free-text entry remains
// possible through the zone config API.
package species

type Species struct {
	ID         string `json:"id"` // botanical ID, e.g. QUMA
	Common     string `json:"common_name"`
	Latin      string `json:"latin_name"`
	Category   string `json:"category"`   // canopy | nurse | riparian | wildlife | specialty | shrub | vine
	Method     string `json:"method"`     // seed | cuttings | hybrid
	Difficulty int    `json:"difficulty"` // 1 (easiest) – 5
}

// Catalog is ordered as the register files it: alphabetical by common name.
var Catalog = []Species{
	{"MATR", "Agarita", "Mahonia trifoliolata", "shrub", "cuttings", 4},
	{"TADI", "Bald Cypress", "Taxodium distichum", "riparian", "hybrid", 1},
	{"ACGR", "Bigtooth Maple", "Acer grandidentatum", "specialty", "seed", 4},
	{"PRSE", "Black Cherry", "Prunus serotina var. eximia", "wildlife", "hybrid", 3},
	{"QUMA", "Bur Oak", "Quercus macrocarpa", "canopy", "seed", 2},
	{"ULCR", "Cedar Elm", "Ulmus crassifolia", "canopy", "seed", 1},
	{"PRAN", "Chickasaw Plum", "Prunus angustifolia", "wildlife", "hybrid", 2},
	{"PODE", "Eastern Cottonwood", "Populus deltoides", "riparian", "cuttings", 1},
	{"FOPU", "Elbowbush", "Forestiera pubescens", "shrub", "cuttings", 3},
	{"RHAR", "Fragrant Sumac", "Rhus aromatica", "shrub", "hybrid", 3},
	{"CELA", "Hackberry", "Celtis laevigata / reticulata", "wildlife", "seed", 1},
	{"QUVI", "Live Oak", "Quercus virginiana", "canopy", "seed", 2},
	{"PRME", "Mexican Plum", "Prunus mexicana", "wildlife", "hybrid", 3},
	{"QUPO", "Monterrey Oak", "Quercus polymorpha", "nurse", "seed", 1},
	{"VIRO", "Muscadine Grape", "Vitis rotundifolia", "vine", "cuttings", 3},
	{"VIMU", "Mustang Grape", "Vitis mustangensis", "vine", "cuttings", 3},
	{"CAIL", "Pecan", "Carya illinoinensis", "riparian", "seed", 2},
	{"QUST", "Post Oak", "Quercus stellata", "canopy", "seed", 2},
	{"ARXA", "Texas Madrone", "Arbutus xalapensis", "specialty", "seed", 5},
	{"DITE", "Texas Persimmon", "Diospyros texana", "wildlife", "hybrid", 2},
	{"ILVO", "Yaupon Holly", "Ilex vomitoria", "shrub", "cuttings", 3},
}
