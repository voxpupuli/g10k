package g10k

func (s *pfParserState) HandleModuleDir(value string) error {
	if len(s.rt.ModuleDir) != 0 {
		return nil
	}
	s.currentModuleDir = normalizeDir(value)
	s.moduleDirs = append(s.moduleDirs, s.currentModuleDir)
	return nil
}
