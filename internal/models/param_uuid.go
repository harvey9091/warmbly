package models

import "github.com/google/uuid"

// ParamUUID is a uuid bound from a query string; gin's form binding cannot set a
// bare uuid.UUID and rejects the whole request instead.
type ParamUUID struct {
	uuid.UUID
}

// UnmarshalParam implements gin's binding.BindUnmarshaler.
func (p *ParamUUID) UnmarshalParam(param string) error {
	id, err := uuid.Parse(param)
	if err != nil {
		return err
	}
	p.UUID = id
	return nil
}
