package events

// resumeRequesterAllowed 判定 issue 评论触发上下文补充(resume)的请求者白名单:
// issue 作者 ∪ 指派人 ∪ 平台级白名单。降级阶梯:
//   - 无指派人时退化为「作者 ∪ 白名单」(拍板示例:没有分派人就只允许创建者);
//   - 作者身份在载荷中不可得(author_id 缺失)时,无法确立「仅作者」边界,
//     fail-closed 拒绝(三平台真实载荷作者恒可得,该分支为防御性收口);
//   - 请求者身份缺失时 fail-closed 拒绝(无法归因的触发不值得放行)。
func resumeRequesterAllowed(requesterID, authorID int, assigneeIDs []int, reworkAllowedUserIDs map[int]struct{}) bool {
	if requesterID <= 0 {
		return false
	}
	if authorID > 0 && requesterID == authorID {
		return true
	}
	for _, assigneeID := range assigneeIDs {
		if assigneeID == requesterID {
			return true
		}
	}
	if _, ok := reworkAllowedUserIDs[requesterID]; ok {
		return true
	}
	return false
}
