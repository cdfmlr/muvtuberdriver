// Package audio implements a controller (AudioController) to the 
// audioview, that is, a websocket server that sends audio to the
// audioview. (and then the audioview will play the audio).
// 
// AudioController tracks the status of the audioview by receiving the  
// report (ReportStart, ReportEnd) from the audioview (via websocket).
// See the Wait method for more details.
package audio
