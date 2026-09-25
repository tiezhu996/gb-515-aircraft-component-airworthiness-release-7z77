package dto

// RegisterAssembly 是登记装配关系的输入契约：ParentCode 是组件部件，
// ChildCode 是要装入的子件。服务层负责拦截自环与绕回自己的环。
type RegisterAssembly struct {
	ParentCode string `json:"parentCode" binding:"max=64"`
	ChildCode  string `json:"childCode" binding:"required,min=2,max=64"`
}
